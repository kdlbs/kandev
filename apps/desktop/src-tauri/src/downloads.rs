use std::{
    collections::HashMap,
    path::PathBuf,
    sync::{
        atomic::{AtomicU64, Ordering},
        Arc, Mutex,
    },
};
use url::Url;

#[derive(Clone, Default)]
pub struct DownloadTracker {
    pending: Arc<Mutex<HashMap<String, PendingDownload>>>,
    next_attempt: Arc<AtomicU64>,
}

#[derive(Clone)]
enum PendingDownload {
    Selecting { attempt: u64 },
    Selected { attempt: u64, destination: PathBuf },
    Transferring { destination: PathBuf },
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DownloadCompletion {
    pub destination: PathBuf,
    pub success: bool,
}

impl DownloadTracker {
    #[cfg(test)]
    pub fn begin(&self, url: &Url, destination: PathBuf) -> bool {
        if destination.as_os_str().is_empty() {
            return false;
        }
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        if pending.contains_key(url.as_str()) {
            return false;
        }
        pending.insert(
            url.as_str().to_string(),
            PendingDownload::Transferring { destination },
        );
        true
    }

    pub fn begin_selection(&self, url: &Url) -> Option<u64> {
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        if pending.contains_key(url.as_str()) {
            return None;
        }
        let attempt = self.next_attempt.fetch_add(1, Ordering::Relaxed);
        pending.insert(
            url.as_str().to_string(),
            PendingDownload::Selecting { attempt },
        );
        Some(attempt)
    }

    pub fn select_destination(&self, url: &Url, attempt: u64, destination: PathBuf) -> bool {
        if destination.as_os_str().is_empty() {
            return false;
        }
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        let Some(download) = pending.get_mut(url.as_str()) else {
            return false;
        };
        if !matches!(download, PendingDownload::Selecting { attempt: current } if *current == attempt)
        {
            return false;
        }
        *download = PendingDownload::Selected {
            attempt,
            destination,
        };
        true
    }

    pub fn cancel_selection(&self, url: &Url, attempt: u64) -> bool {
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        let is_attempt = matches!(
            pending.get(url.as_str()),
            Some(PendingDownload::Selecting { attempt: current }) if *current == attempt
        );
        if is_attempt {
            pending.remove(url.as_str());
        }
        is_attempt
    }

    pub fn expire_selection(&self, url: &Url, attempt: u64) -> bool {
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        let is_attempt = matches!(
            pending.get(url.as_str()),
            Some(PendingDownload::Selecting { attempt: current })
                | Some(PendingDownload::Selected { attempt: current, .. })
                if *current == attempt
        );
        if is_attempt {
            pending.remove(url.as_str());
        }
        is_attempt
    }

    pub fn start_selected_transfer(&self, url: &Url) -> Option<PathBuf> {
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        let download = pending.get_mut(url.as_str())?;
        let PendingDownload::Selected { destination, .. } = download else {
            return None;
        };
        let destination = destination.clone();
        *download = PendingDownload::Transferring {
            destination: destination.clone(),
        };
        Some(destination)
    }

    pub fn finish(&self, url: &Url, success: bool) -> Option<DownloadCompletion> {
        let mut pending = self
            .pending
            .lock()
            .expect("download tracker mutex poisoned");
        let destination = match pending.get(url.as_str())? {
            PendingDownload::Transferring { destination } => destination.clone(),
            _ => return None,
        };
        pending.remove(url.as_str());
        Some(DownloadCompletion {
            destination,
            success,
        })
    }

    #[cfg(test)]
    fn pending_count(&self) -> usize {
        self.pending
            .lock()
            .expect("download tracker mutex poisoned")
            .len()
    }
}

fn is_owned_download(
    backend: &crate::backend::BackendState,
    page_url: &Url,
    download_url: &Url,
) -> bool {
    backend.accepts_url(page_url.as_str()) && backend.accepts_url(download_url.as_str())
}

fn safe_suggested_filename(suggested_path: &std::path::Path) -> Option<String> {
    let suggestion = suggested_path.to_string_lossy();
    let name = suggestion.rsplit(['/', '\\']).next()?;
    if name.is_empty() {
        return None;
    }

    let cleaned: String = name
        .chars()
        .take(180)
        .map(|character| {
            if character.is_control() || "<>:\"|?*".contains(character) {
                '_'
            } else {
                character
            }
        })
        .collect();
    let cleaned = cleaned.trim().trim_end_matches('.');
    if cleaned.is_empty() || cleaned == "." || cleaned == ".." {
        None
    } else {
        Some(cleaned.to_string())
    }
}

#[cfg(feature = "desktop-runtime")]
mod runtime {
    use super::{is_owned_download, safe_suggested_filename, DownloadTracker};
    use crate::backend::BackendState;
    use serde::Serialize;
    use std::time::Duration;
    use tauri::{webview::DownloadEvent, AppHandle, Emitter, Manager, WebviewWindow};
    use tauri_plugin_dialog::DialogExt;
    use url::Url;

    const MAIN_WINDOW_LABEL: &str = "main";
    const DOWNLOAD_FEEDBACK_EVENT: &str = "kandev-desktop-v1-download";
    const DOWNLOAD_READY_EVENT: &str = "kandev-desktop-v1-download-ready";
    const SELECTION_TIMEOUT: Duration = Duration::from_secs(30 * 60);

    #[derive(Clone, Serialize)]
    #[serde(rename_all = "camelCase")]
    enum DownloadStatus {
        Selecting,
        Started,
        Saved,
        Cancelled,
        Failed,
    }

    #[derive(Clone, Serialize)]
    #[serde(rename_all = "camelCase")]
    struct DownloadFeedback {
        status: DownloadStatus,
        url: String,
        file_name: String,
    }

    #[derive(Clone, Serialize)]
    #[serde(rename_all = "camelCase")]
    struct DownloadReady {
        url: String,
        file_name: String,
    }

    fn emit_feedback(window: &WebviewWindow, url: &Url, file_name: &str, status: DownloadStatus) {
        let _ = window.emit(
            DOWNLOAD_FEEDBACK_EVENT,
            DownloadFeedback {
                status,
                url: url.as_str().to_string(),
                file_name: file_name.to_string(),
            },
        );
    }

    fn emit_download_ready(window: &WebviewWindow, url: &Url, file_name: &str) {
        let _ = window.emit(
            DOWNLOAD_READY_EVENT,
            DownloadReady {
                url: url.as_str().to_string(),
                file_name: file_name.to_string(),
            },
        );
    }

    pub fn handle_download_event(
        app: &AppHandle,
        backend: &BackendState,
        tracker: &DownloadTracker,
        event: DownloadEvent<'_>,
    ) -> bool {
        let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) else {
            return false;
        };

        match event {
            DownloadEvent::Requested { url, destination } => {
                let Ok(page_url) = window.url() else {
                    return false;
                };
                if !backend.accepts_url(page_url.as_str()) {
                    return false;
                }
                let Some(file_name) = safe_suggested_filename(destination) else {
                    emit_feedback(&window, &url, "download", DownloadStatus::Failed);
                    return false;
                };
                if backend.require_owned_origin(&window).is_err() {
                    emit_feedback(&window, &url, &file_name, DownloadStatus::Failed);
                    return false;
                }
                if !is_owned_download(backend, &page_url, &url) {
                    emit_feedback(&window, &url, &file_name, DownloadStatus::Failed);
                    return false;
                }
                if let Some(selected_path) = tracker.start_selected_transfer(&url) {
                    *destination = selected_path;
                    emit_feedback(&window, &url, &file_name, DownloadStatus::Started);
                    return true;
                }

                let Some(attempt) = tracker.begin_selection(&url) else {
                    emit_feedback(&window, &url, &file_name, DownloadStatus::Failed);
                    return false;
                };
                emit_feedback(&window, &url, &file_name, DownloadStatus::Selecting);
                let callback_window = window.clone();
                let callback_tracker = tracker.clone();
                let callback_url = url.clone();
                let callback_file_name = file_name.clone();
                let timeout_tracker = tracker.clone();
                let timeout_url = url.clone();
                let timeout_window = window.clone();
                let timeout_file_name = file_name.clone();
                std::thread::spawn(move || {
                    std::thread::sleep(SELECTION_TIMEOUT);
                    if timeout_tracker.expire_selection(&timeout_url, attempt) {
                        emit_feedback(
                            &timeout_window,
                            &timeout_url,
                            &timeout_file_name,
                            DownloadStatus::Failed,
                        );
                    }
                });
                app.dialog()
                    .file()
                    .set_file_name(&file_name)
                    .set_parent(&window)
                    .save_file(move |selected| {
                        let Some(selected) = selected else {
                            callback_tracker.cancel_selection(&callback_url, attempt);
                            emit_feedback(
                                &callback_window,
                                &callback_url,
                                &callback_file_name,
                                DownloadStatus::Cancelled,
                            );
                            return;
                        };
                        let selected_path = match selected.into_path() {
                            Ok(path) => path,
                            Err(_) => {
                                callback_tracker.cancel_selection(&callback_url, attempt);
                                emit_feedback(
                                    &callback_window,
                                    &callback_url,
                                    &callback_file_name,
                                    DownloadStatus::Failed,
                                );
                                return;
                            }
                        };
                        if callback_tracker.select_destination(
                            &callback_url,
                            attempt,
                            selected_path,
                        ) {
                            emit_download_ready(
                                &callback_window,
                                &callback_url,
                                &callback_file_name,
                            );
                        } else {
                            callback_tracker.cancel_selection(&callback_url, attempt);
                            emit_feedback(
                                &callback_window,
                                &callback_url,
                                &callback_file_name,
                                DownloadStatus::Failed,
                            );
                        }
                    });
                false
            }
            DownloadEvent::Finished { url, success, .. } => {
                if let Some(completion) = tracker.finish(&url, success) {
                    let file_name = completion
                        .destination
                        .file_name()
                        .and_then(|name| name.to_str())
                        .unwrap_or("download");
                    let status = if completion.success {
                        DownloadStatus::Saved
                    } else {
                        DownloadStatus::Failed
                    };
                    if window
                        .url()
                        .is_ok_and(|page_url| backend.accepts_url(page_url.as_str()))
                    {
                        emit_feedback(&window, &url, file_name, status);
                    }
                }
                true
            }
            _ => true,
        }
    }
}

#[cfg(feature = "desktop-runtime")]
pub use runtime::handle_download_event;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::BackendState;
    use std::path::PathBuf;
    use url::Url;

    const OWNED_ORIGIN: &str = "http://127.0.0.1:38430";

    fn owned_backend() -> BackendState {
        let backend = BackendState::default();
        backend.set_owned_origin(OWNED_ORIGIN).unwrap();
        backend
    }

    #[test]
    fn accepts_owned_http_and_blob_downloads_from_the_owned_page() {
        let page = Url::parse("http://127.0.0.1:38430/settings/system/logs").unwrap();
        let http = Url::parse("http://127.0.0.1:38430/api/logs.zip").unwrap();
        let blob = Url::parse("blob:http://127.0.0.1:38430/1234-5678").unwrap();
        let backend = owned_backend();

        assert!(is_owned_download(&backend, &page, &http));
        assert!(is_owned_download(&backend, &page, &blob));
    }

    #[test]
    fn rejects_untrusted_pages_and_download_origins() {
        let owned_page = Url::parse("http://127.0.0.1:38430/settings").unwrap();
        let remote_page = Url::parse("https://example.com").unwrap();
        let owned_download = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let remote_download = Url::parse("https://example.com/export.zip").unwrap();
        let remote_blob = Url::parse("blob:https://example.com/1234").unwrap();
        let backend = owned_backend();

        assert!(!is_owned_download(&backend, &remote_page, &owned_download));
        assert!(!is_owned_download(&backend, &owned_page, &remote_download));
        assert!(!is_owned_download(&backend, &owned_page, &remote_blob));
    }

    #[test]
    fn sanitizes_a_suggested_name_without_accepting_path_components() {
        assert_eq!(
            safe_suggested_filename(&PathBuf::from(r"C:\private\..\logs.zip")),
            Some("logs.zip".to_string())
        );
        assert_eq!(
            safe_suggested_filename(&PathBuf::from("/tmp/diagnostic\nlogs.zip")),
            Some("diagnostic_logs.zip".to_string())
        );
        assert_eq!(safe_suggested_filename(&PathBuf::from("/tmp/..")), None);
    }

    #[test]
    fn completion_uses_the_selected_destination_when_the_engine_omits_its_path() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let selected = PathBuf::from("/Users/example/Documents/export.zip");

        assert!(tracker.begin(&url, selected.clone()));
        assert_eq!(
            tracker.finish(&url, true),
            Some(DownloadCompletion {
                destination: selected,
                success: true,
            })
        );
        assert_eq!(tracker.pending_count(), 0);
    }

    #[test]
    fn failed_completion_releases_the_url_for_a_retry() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let selected = PathBuf::from("/Users/example/Documents/export.zip");

        assert!(tracker.begin(&url, selected.clone()));
        assert_eq!(
            tracker.finish(&url, false),
            Some(DownloadCompletion {
                destination: selected.clone(),
                success: false,
            })
        );
        assert!(tracker.begin(&url, selected));
    }

    #[test]
    fn rejects_a_transfer_without_a_selected_destination() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();

        assert!(!tracker.begin(&url, PathBuf::new()));
        assert_eq!(tracker.pending_count(), 0);
    }

    #[test]
    fn rejects_an_empty_save_dialog_destination() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let attempt = tracker.begin_selection(&url).unwrap();

        assert!(!tracker.select_destination(&url, attempt, PathBuf::new()));
        assert!(tracker.cancel_selection(&url, attempt));
        assert_eq!(tracker.pending_count(), 0);
    }

    #[test]
    fn native_download_hook_uses_the_nonblocking_save_dialog_api() {
        let source = include_str!("downloads.rs")
            .split("#[cfg(test)]\nmod tests")
            .next()
            .unwrap();

        assert!(source.contains(".save_file("));
        assert!(!source.contains(".blocking_save_file("));
    }

    #[test]
    fn selected_destination_is_used_for_the_retried_request_once() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let destination = PathBuf::from("/tmp/export.zip");
        let attempt = tracker.begin_selection(&url).unwrap();

        assert!(tracker.select_destination(&url, attempt, destination.clone()));
        assert_eq!(
            tracker.start_selected_transfer(&url),
            Some(destination.clone())
        );
        assert_eq!(tracker.start_selected_transfer(&url), None);
        assert_eq!(tracker.finish(&url, true).unwrap().destination, destination);
    }

    #[test]
    fn cancelled_and_expired_selections_release_the_url_for_retry() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let cancelled = tracker.begin_selection(&url).unwrap();
        assert!(tracker.cancel_selection(&url, cancelled));

        let expired = tracker.begin_selection(&url).unwrap();
        assert!(tracker.select_destination(&url, expired, PathBuf::from("/tmp/export.zip")));
        assert!(tracker.expire_selection(&url, expired));
        assert_eq!(tracker.pending_count(), 0);
        assert!(tracker.begin_selection(&url).is_some());
    }

    #[test]
    fn an_old_selection_timeout_cannot_remove_a_newer_retry() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();
        let expired = tracker.begin_selection(&url).unwrap();
        assert!(tracker.cancel_selection(&url, expired));

        let retry = tracker.begin_selection(&url).unwrap();
        assert!(!tracker.expire_selection(&url, expired));
        assert!(tracker.select_destination(&url, retry, PathBuf::from("/tmp/retry.zip")));
    }

    #[test]
    fn concurrent_requests_for_the_same_url_cannot_mix_destinations() {
        let tracker = DownloadTracker::default();
        let url = Url::parse("http://127.0.0.1:38430/export.zip").unwrap();

        assert!(tracker.begin(&url, PathBuf::from("/tmp/first.zip")));
        assert!(!tracker.begin(&url, PathBuf::from("/tmp/second.zip")));
        assert_eq!(
            tracker.finish(&url, true).unwrap().destination,
            PathBuf::from("/tmp/first.zip")
        );
        assert!(tracker.begin(&url, PathBuf::from("/tmp/second.zip")));
    }

    #[test]
    fn simultaneous_http_and_blob_completions_keep_their_selected_destinations() {
        let tracker = DownloadTracker::default();
        let http = Url::parse("http://127.0.0.1:38430/logs.zip").unwrap();
        let blob = Url::parse("blob:http://127.0.0.1:38430/1234-5678").unwrap();
        let http_path = PathBuf::from("/tmp/logs.zip");
        let blob_path = PathBuf::from("/tmp/chart.svg");

        assert!(tracker.begin(&http, http_path.clone()));
        assert!(tracker.begin(&blob, blob_path.clone()));
        assert_eq!(tracker.finish(&blob, true).unwrap().destination, blob_path);
        assert_eq!(tracker.finish(&http, true).unwrap().destination, http_path);
    }
}
