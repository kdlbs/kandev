use serde::Serialize;
use std::path::{Path, PathBuf};

const START_DIRECTORY_CANDIDATES: [&str; 7] = [
    "Projects",
    "Developer",
    "src",
    "Code",
    "workspace",
    "Development",
    "repos",
];

/// The result of the deliberately narrow native folder-selection boundary.
/// The command never accepts a path from the WebView. The selected path is
/// returned only after the user chooses it in the operating-system picker.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(tag = "status", rename_all = "camelCase")]
pub enum FolderPickerOutcome {
    Selected { path: String },
    Cancelled,
    Failed { message: String },
}

fn outcome_from_path(path: Option<Result<PathBuf, String>>) -> FolderPickerOutcome {
    match path {
        Some(Ok(path)) => FolderPickerOutcome::Selected {
            path: path.to_string_lossy().into_owned(),
        },
        Some(Err(message)) => FolderPickerOutcome::Failed { message },
        None => FolderPickerOutcome::Cancelled,
    }
}

fn workspace_start_directory(home: &Path) -> Option<PathBuf> {
    START_DIRECTORY_CANDIDATES.iter().find_map(|name| {
        let candidate = home.join(name);
        let metadata = std::fs::symlink_metadata(&candidate).ok()?;
        (metadata.is_dir() && !metadata.file_type().is_symlink()).then_some(candidate)
    })
}

#[cfg(feature = "desktop-runtime")]
async fn receive_picker_outcome(
    mut receiver: tauri::async_runtime::Receiver<Option<Result<PathBuf, String>>>,
) -> FolderPickerOutcome {
    match receiver.recv().await {
        Some(path) => outcome_from_path(path),
        None => FolderPickerOutcome::Failed {
            message: "native folder picker did not return a result".to_string(),
        },
    }
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub async fn pick_directory(
    app: tauri::AppHandle,
    backend: tauri::State<'_, crate::backend::BackendState>,
    webview: tauri::WebviewWindow,
) -> Result<FolderPickerOutcome, String> {
    use tauri_plugin_dialog::{DialogExt, FilePath};

    backend.require_owned_origin(&webview)?;
    let home = crate::backend::picker_home_dir()
        .ok_or_else(|| "could not determine the desktop user's home directory".to_string())?;
    let mut dialog = app.dialog().file();
    if let Some(start_directory) = workspace_start_directory(&home) {
        dialog = dialog.set_directory(start_directory);
    }
    let (sender, receiver) = tauri::async_runtime::channel(1);
    dialog.set_parent(&webview).pick_folder(move |picked| {
        let result = picked.map(|path: FilePath| path.into_path().map_err(|err| err.to_string()));
        let _ = sender.try_send(result);
    });
    Ok(receive_picker_outcome(receiver).await)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn maps_a_selected_directory() {
        assert_eq!(
            outcome_from_path(Some(Ok(PathBuf::from("/Users/example/Code")))),
            FolderPickerOutcome::Selected {
                path: "/Users/example/Code".to_string()
            }
        );
    }

    #[test]
    fn maps_cancellation_without_a_path() {
        assert_eq!(outcome_from_path(None), FolderPickerOutcome::Cancelled);
    }

    #[test]
    fn maps_picker_failures_without_exposing_a_path_argument() {
        assert_eq!(
            outcome_from_path(Some(Err("picker unavailable".to_string()))),
            FolderPickerOutcome::Failed {
                message: "picker unavailable".to_string()
            }
        );
    }

    #[test]
    fn chooses_the_first_existing_workspace_directory_in_order() {
        let home = test_home();
        let first = home.join("src");
        let later = home.join("Code");
        fs::create_dir_all(&first).expect("create src directory");
        fs::create_dir_all(&later).expect("create Code directory");

        assert_eq!(workspace_start_directory(&home), Some(first));
        fs::remove_dir_all(home).expect("remove test home");
    }

    #[test]
    fn skips_missing_folders_and_returns_none_when_no_candidate_exists() {
        let home = test_home();

        assert_eq!(workspace_start_directory(&home), None);
        fs::remove_dir_all(home).expect("remove test home");
    }

    #[test]
    fn rejects_symlink_candidates_and_continues_to_the_next_directory() {
        let home = test_home();
        let linked = home.join("Projects");
        let target = home.join("linked-target");
        let next = home.join("Developer");
        fs::create_dir_all(&target).expect("create target directory");
        fs::create_dir_all(&next).expect("create Developer directory");
        #[cfg(unix)]
        std::os::unix::fs::symlink(&target, &linked).expect("create Projects symlink");
        #[cfg(windows)]
        std::os::windows::fs::symlink_dir(&target, &linked).expect("create Projects symlink");

        assert_eq!(workspace_start_directory(&home), Some(next));
        fs::remove_dir_all(home).expect("remove test directories");
    }

    #[test]
    fn maps_selection_cancellation_and_closed_callback_channel() {
        let selection = async_channel_outcome(Some(Ok(PathBuf::from("/Users/example/Code"))));
        assert_eq!(
            selection,
            FolderPickerOutcome::Selected {
                path: "/Users/example/Code".to_string()
            }
        );
        assert_eq!(async_channel_outcome(None), FolderPickerOutcome::Cancelled);

        let (sender, receiver) = tauri::async_runtime::channel(1);
        drop(sender);
        assert_eq!(
            tauri::async_runtime::block_on(receive_picker_outcome(receiver)),
            FolderPickerOutcome::Failed {
                message: "native folder picker did not return a result".to_string()
            }
        );
    }

    fn async_channel_outcome(value: Option<Result<PathBuf, String>>) -> FolderPickerOutcome {
        let (sender, receiver) = tauri::async_runtime::channel(1);
        sender
            .try_send(value)
            .expect("send simulated picker result");
        drop(sender);
        tauri::async_runtime::block_on(receive_picker_outcome(receiver))
    }

    fn test_home() -> PathBuf {
        use std::time::{SystemTime, UNIX_EPOCH};

        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system time after Unix epoch")
            .as_nanos();
        let home = std::env::temp_dir().join(format!(
            "kandev-folder-picker-{}-{unique}",
            std::process::id()
        ));
        fs::create_dir_all(&home).expect("create test home");
        home
    }
}
