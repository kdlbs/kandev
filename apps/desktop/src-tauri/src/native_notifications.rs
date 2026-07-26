use serde::{Deserialize, Serialize};
use std::{collections::VecDeque, sync::Mutex};

const MAX_SEEN_EVENT_IDS: usize = 1_024;

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NativeNotificationRequest {
    pub event_id: String,
    pub title: String,
    pub body: String,
    /// Present for session-scoped events (turn-finished, clarification-requested, failed).
    /// Absent for account/app-scoped events such as an available update,
    /// which has no associated task. `#[serde(default)]` accepts payloads
    /// that omit the key entirely (e.g. `JSON.stringify` drops `undefined`).
    #[serde(default)]
    pub task_id: Option<String>,
    pub session_id: Option<String>,
}
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum NativeNotificationResult {
    Shown,
    Duplicate,
    PermissionDenied,
    DisplayFailed,
}

#[derive(Debug, Default)]
pub struct NativeNotificationState {
    seen_event_ids: Mutex<VecDeque<String>>,
}

impl NativeNotificationState {
    fn claim(&self, event_id: &str) -> bool {
        let mut seen = self
            .seen_event_ids
            .lock()
            .expect("notification identity mutex poisoned");
        if seen.iter().any(|seen_id| seen_id == event_id) {
            return false;
        }
        if seen.len() == MAX_SEEN_EVENT_IDS {
            seen.pop_front();
        }
        seen.push_back(event_id.to_string());
        true
    }

    fn release(&self, event_id: &str) {
        self.seen_event_ids
            .lock()
            .expect("notification identity mutex poisoned")
            .retain(|seen_id| seen_id != event_id);
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
}

/// Event identities this bridge will display. Session-scoped events require
/// a non-empty task_id (they're gated on task context); the update-available
/// event has no task and must omit it.
const SESSION_EVENT_PREFIXES: [&str; 3] = [
    "session.failed:",
    "session.turn_finished:",
    "session.clarification_requested:",
];
const UPDATE_AVAILABLE_PREFIX: &str = "system.update_available:";

fn validate_request(request: &NativeNotificationRequest) -> Result<(), String> {
    let is_session_event = SESSION_EVENT_PREFIXES
        .iter()
        .any(|prefix| request.event_id.starts_with(prefix));
    let is_update_event = request.event_id.starts_with(UPDATE_AVAILABLE_PREFIX);
    if !is_session_event && !is_update_event {
        return Err("unsupported native notification event".to_string());
    }
    if request.event_id.len() > 256
        || request.title.is_empty()
        || request.title.len() > 160
        || request.body.len() > 1000
        || request.session_id.as_ref().is_some_and(|id| id.len() > 256)
        || request.task_id.as_ref().is_some_and(|id| id.len() > 256)
    {
        return Err("invalid native notification payload".to_string());
    }
    if is_session_event {
        match request.task_id.as_deref() {
            Some(id) if !id.is_empty() => {}
            _ => return Err("invalid native notification payload".to_string()),
        }
    }
    Ok(())
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn show_native_notification(
    app: tauri::AppHandle,
    state: tauri::State<'_, NativeNotificationState>,
    backend: tauri::State<'_, crate::backend::BackendState>,
    webview: tauri::WebviewWindow,
    request: NativeNotificationRequest,
) -> Result<NativeNotificationResult, String> {
    use tauri::plugin::PermissionState;
    use tauri_plugin_notification::NotificationExt;

    backend.require_owned_origin(&webview)?;
    validate_request(&request)?;
    let permission = app
        .notification()
        .permission_state()
        .map_err(|err| err.to_string())?;
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
        eprintln!("Could not show native notification: {err}");
        return Ok(NativeNotificationResult::DisplayFailed);
    }
    Ok(NativeNotificationResult::Shown)
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn get_native_notification_permission(
    app: tauri::AppHandle,
    backend: tauri::State<'_, crate::backend::BackendState>,
    webview: tauri::WebviewWindow,
) -> Result<tauri::plugin::PermissionState, String> {
    use tauri_plugin_notification::NotificationExt;

    backend.require_owned_origin(&webview)?;
    app.notification()
        .permission_state()
        .map_err(|err| err.to_string())
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn request_native_notification_permission(
    app: tauri::AppHandle,
    backend: tauri::State<'_, crate::backend::BackendState>,
    webview: tauri::WebviewWindow,
) -> Result<tauri::plugin::PermissionState, String> {
    use tauri_plugin_notification::NotificationExt;

    backend.require_owned_origin(&webview)?;
    app.notification()
        .request_permission()
        .map_err(|err| err.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn request(event_id: &str, task_id: &str) -> NativeNotificationRequest {
        NativeNotificationRequest {
            event_id: event_id.to_string(),
            title: "Agent needs your answer".to_string(),
            body: "The agent asked a question.".to_string(),
            task_id: Some(task_id.to_string()),
            session_id: Some("session-1".to_string()),
        }
    }

    fn update_available_request(event_id: &str) -> NativeNotificationRequest {
        NativeNotificationRequest {
            event_id: event_id.to_string(),
            title: "Update available".to_string(),
            body: "Kandev v1.2.3 is available.".to_string(),
            task_id: None,
            session_id: None,
        }
    }

    #[test]
    fn event_identity_is_delivered_at_most_once() {
        let state = NativeNotificationState::default();
        let request = request("session.clarification_requested:session-1", "task-1");

        assert!(state.claim(&request.event_id));
        assert!(!state.claim(&request.event_id));
    }

    #[test]
    fn event_identity_cache_is_bounded() {
        let state = NativeNotificationState::default();

        for index in 0..=MAX_SEEN_EVENT_IDS {
            assert!(state.claim(&format!("session.failed:{index}")));
        }

        assert!(state.claim("session.failed:0"));
    }

    #[test]
    fn bridge_rejects_events_outside_supported_notification_types() {
        let request = request("office.inbox_item:item-1", "task-1");

        assert_eq!(
            validate_request(&request),
            Err("unsupported native notification event".to_string())
        );
    }

    #[test]
    fn bridge_accepts_semantic_session_events() {
        for event_id in [
            "session.turn_finished:turn-1",
            "session.clarification_requested:request-1",
            "session.failed:session-1",
        ] {
            assert_eq!(validate_request(&request(event_id, "task-1")), Ok(()));
        }
    }

    #[test]
    fn bridge_rejects_retired_waiting_for_input_events() {
        let request = request("session.waiting_for_input:session-1", "task-1");

        assert_eq!(
            validate_request(&request),
            Err("unsupported native notification event".to_string())
        );
    }

    #[test]
    fn bridge_accepts_update_available_events_without_task_id() {
        let request = update_available_request("system.update_available:v1.2.3");

        assert_eq!(validate_request(&request), Ok(()));
    }

    #[test]
    fn bridge_rejects_session_events_missing_task_id() {
        let mut request = request("session.clarification_requested:session-1", "task-1");
        request.task_id = None;

        assert_eq!(
            validate_request(&request),
            Err("invalid native notification payload".to_string())
        );
    }

    #[test]
    fn update_available_identity_is_delivered_at_most_once() {
        let state = NativeNotificationState::default();
        let request = update_available_request("system.update_available:v1.2.3");

        assert!(state.claim(&request.event_id));
        assert!(!state.claim(&request.event_id));
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
    fn display_failure_releases_the_event_identity_for_retry() {
        let state = NativeNotificationState::default();
        let request = request("session.failed:session-1", "task-1");

        assert!(state.claim(&request.event_id));
        state.release(&request.event_id);
        assert_eq!(
            state.claim_after_permission(&request, true),
            NativeNotificationResult::Shown
        );
    }
}
