use kandev_desktop::backend;
use tauri::{Manager, WindowEvent};

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .manage(backend::BackendState::default())
        .setup(|app| {
            let window = app
                .get_webview_window("main")
                .expect("main window should exist");
            backend::start_desktop_backend(app.handle().clone(), window);
            Ok(())
        })
        .on_window_event(|window, event| {
            if matches!(event, WindowEvent::CloseRequested { .. }) {
                window.app_handle().state::<backend::BackendState>().stop();
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running Kandev desktop app");
}
