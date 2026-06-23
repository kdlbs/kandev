use kandev_desktop::backend;
use std::thread;
use tauri::{Manager, RunEvent, WindowEvent};

fn main() {
    let app = tauri::Builder::default()
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
            if let WindowEvent::CloseRequested { api, .. } = event {
                let state = window.app_handle().state::<backend::BackendState>().inner().clone();
                if state.begin_shutdown() {
                    api.prevent_close();
                    let window = window.clone();
                    thread::spawn(move || {
                        state.stop();
                        let _ = window.close();
                    });
                }
            }
        })
        .build(tauri::generate_context!())
        .expect("error while building Kandev desktop app");

    app.run(|app_handle, event| {
        if let RunEvent::ExitRequested { api, .. } = event {
            let state = app_handle.state::<backend::BackendState>().inner().clone();
            if state.begin_shutdown() {
                api.prevent_exit();
                let app_handle = app_handle.clone();
                thread::spawn(move || {
                    state.stop();
                    app_handle.exit(0);
                });
            }
        }
    });
}
