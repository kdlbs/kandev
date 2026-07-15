use serde::{Deserialize, Serialize};
use std::{
    collections::HashSet,
    sync::{
        atomic::{AtomicBool, Ordering},
        Mutex,
    },
};

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NativeNotificationRequest {
    pub event_id: String,
    pub title: String,
    pub body: String,
    pub task_id: String,
    pub session_id: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum NativeNotificationResult {
    Shown,
    Duplicate,
    PermissionDenied,
}

#[derive(Debug, Default)]
pub struct NativeNotificationState {
    seen_event_ids: Mutex<HashSet<String>>,
    permission_prompt_attempted: AtomicBool,
}

impl NativeNotificationState {
    fn claim(&self, event_id: &str) -> bool {
        let mut seen = self
            .seen_event_ids
            .lock()
            .expect("notification identity mutex poisoned");
        seen.insert(event_id.to_string())
    }

    fn release(&self, event_id: &str) {
        self.seen_event_ids
            .lock()
            .expect("notification identity mutex poisoned")
            .remove(event_id);
    }

    fn claim_after_permission(
        &self,
        request: &NativeNotificationRequest,
        permission_granted: bool,
    ) -> NativeNotificationResult {
        if !permission_granted {
            return NativeNotificationResult::PermissionDenied;
        }
        if self.claim(&request.event_id) {
            NativeNotificationResult::Shown
        } else {
            NativeNotificationResult::Duplicate
        }
    }

    fn begin_permission_prompt(&self) -> bool {
        !self
            .permission_prompt_attempted
            .swap(true, Ordering::SeqCst)
    }
}

fn validate_request(request: &NativeNotificationRequest) -> Result<(), String> {
    if !request.event_id.starts_with("session.waiting_for_input:")
        && !request.event_id.starts_with("session.failed:")
    {
        return Err("unsupported native notification event".to_string());
    }
    if request.event_id.len() > 256
        || request.title.is_empty()
        || request.title.len() > 160
        || request.body.len() > 1000
        || request.task_id.is_empty()
        || request.task_id.len() > 256
        || request.session_id.as_ref().is_some_and(|id| id.len() > 256)
    {
        return Err("invalid native notification payload".to_string());
    }
    Ok(())
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn show_native_notification(
    app: tauri::AppHandle,
    state: tauri::State<'_, NativeNotificationState>,
    request: NativeNotificationRequest,
) -> Result<NativeNotificationResult, String> {
    use tauri::plugin::PermissionState;
    use tauri_plugin_notification::NotificationExt;

    validate_request(&request)?;
    let mut permission = app
        .notification()
        .permission_state()
        .map_err(|err| err.to_string())?;
    if permission != PermissionState::Granted
        && permission != PermissionState::Denied
        && state.begin_permission_prompt()
    {
        permission = app
            .notification()
            .request_permission()
            .map_err(|err| err.to_string())?;
    }
    let claim_result =
        state.claim_after_permission(&request, permission == PermissionState::Granted);
    if claim_result != NativeNotificationResult::Shown {
        return Ok(claim_result);
    }
    let show_result = app
        .notification()
        .builder()
        .title(&request.title)
        .body(&request.body)
        .show();
    if let Err(err) = show_result {
        state.release(&request.event_id);
        return Err(err.to_string());
    }
    Ok(NativeNotificationResult::Shown)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn request(event_id: &str, task_id: &str) -> NativeNotificationRequest {
        NativeNotificationRequest {
            event_id: event_id.to_string(),
            title: "Task needs your input".to_string(),
            body: "Waiting".to_string(),
            task_id: task_id.to_string(),
            session_id: Some("session-1".to_string()),
        }
    }

    #[test]
    fn event_identity_is_delivered_at_most_once() {
        let state = NativeNotificationState::default();
        let request = request("session.waiting_for_input:session-1", "task-1");

        assert!(state.claim(&request.event_id));
        assert!(!state.claim(&request.event_id));
    }

    #[test]
    fn bridge_rejects_events_outside_the_two_notification_types() {
        let request = request("office.inbox_item:item-1", "task-1");

        assert_eq!(
            validate_request(&request),
            Err("unsupported native notification event".to_string())
        );
    }

    #[test]
    fn denied_delivery_does_not_claim_the_event() {
        let state = NativeNotificationState::default();
        let request = request("session.failed:session-1", "task-1");

        assert_eq!(
            state.claim_after_permission(&request, false),
            NativeNotificationResult::PermissionDenied
        );
        assert_eq!(
            state.claim_after_permission(&request, true),
            NativeNotificationResult::Shown
        );
    }

    #[test]
    fn permission_prompt_is_attempted_only_once_per_process() {
        let state = NativeNotificationState::default();

        assert!(state.begin_permission_prompt());
        assert!(!state.begin_permission_prompt());
    }
}
