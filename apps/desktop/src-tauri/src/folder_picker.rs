use serde::Serialize;
use std::path::PathBuf;

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

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn pick_directory(
    app: tauri::AppHandle,
    backend: tauri::State<'_, crate::backend::BackendState>,
    webview: tauri::WebviewWindow,
) -> Result<FolderPickerOutcome, String> {
    use tauri_plugin_dialog::{DialogExt, FilePath};

    backend.require_owned_origin(&webview)?;
    let home = crate::backend::picker_home_dir()
        .ok_or_else(|| "could not determine the desktop user's home directory".to_string())?;
    let picked = app
        .dialog()
        .file()
        .set_directory(home)
        .set_parent(&webview)
        .blocking_pick_folder();
    let result = picked.map(|path: FilePath| path.into_path().map_err(|err| err.to_string()));
    Ok(outcome_from_path(result))
}

#[cfg(test)]
mod tests {
    use super::*;

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
}
