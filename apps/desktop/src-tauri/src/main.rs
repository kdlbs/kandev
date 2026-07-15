use kandev_desktop::{
    backend, external_links,
    native_notifications::{self, NativeNotificationState},
    shell::{self, MenuAction, ZoomState},
    updater::{self, UpdaterState},
    window_state::WindowStateStore,
};
use std::thread;
use tauri::{
    menu::{Menu, MenuItemBuilder, PredefinedMenuItem, Submenu},
    Emitter, Manager, RunEvent, WindowEvent,
};

const MAIN_WINDOW_LABEL: &str = "main";
const WINDOW_STATE_FILE: &str = "window-state-v1.json";
#[cfg(target_os = "macos")]
const FULLSCREEN_ACCELERATOR: &str = "Ctrl+Cmd+F";
#[cfg(not(target_os = "macos"))]
const FULLSCREEN_ACCELERATOR: &str = "F11";

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_notification::init())
        .plugin(
            tauri_plugin_opener::Builder::new()
                .open_js_links_on_click(false)
                .build(),
        )
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| {
            activate_main_window(app);
        }))
        .manage(backend::BackendState::default())
        .manage(UpdaterState::new(env!("CARGO_PKG_VERSION")))
        .manage(NativeNotificationState::default())
        .manage(ZoomState::default())
        .invoke_handler(tauri::generate_handler![
            updater::get_update_state,
            updater::check_for_updates,
            updater::install_update,
            native_notifications::show_native_notification,
            external_links::open_external_url,
        ])
        .menu(build_menu)
        .on_menu_event(handle_menu_event)
        .setup(|app| {
            let window = app
                .get_webview_window(MAIN_WINDOW_LABEL)
                .expect("main window should exist");
            let state_path = app.path().app_data_dir()?.join(WINDOW_STATE_FILE);
            let window_state = WindowStateStore::new(state_path);
            if let Err(err) = window_state.restore(&window) {
                eprintln!("Could not restore desktop window state: {err}");
            }
            if let Err(err) = window_state.save(&window) {
                eprintln!("Could not initialize desktop window state: {err}");
            }
            app.manage(window_state);
            window.show()?;
            backend::start_desktop_backend(app.handle().clone(), window);
            Ok(())
        })
        .on_window_event(|window, event| match event {
            WindowEvent::Moved(_) | WindowEvent::Resized(_) => {
                if let Some(webview) = window.app_handle().get_webview_window(window.label()) {
                    if let Some(state) = window.try_state::<WindowStateStore>() {
                        if let Err(err) = state.schedule_save(&webview) {
                            eprintln!("Could not schedule desktop window state: {err}");
                        }
                    }
                }
            }
            WindowEvent::CloseRequested { api, .. } => {
                api.prevent_close();
                if let Some(webview) = window.app_handle().get_webview_window(window.label()) {
                    if let Some(state) = window.try_state::<WindowStateStore>() {
                        let _ = state.save(&webview);
                    }
                }
                shutdown_and_exit(window.app_handle());
            }
            _ => {}
        })
        .build(tauri::generate_context!())
        .expect("error while building Kandev desktop app");

    app.run(|app_handle, event| match event {
        RunEvent::ExitRequested { code, api, .. } if code.is_none() => {
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
        #[cfg(target_os = "macos")]
        RunEvent::Reopen { .. } => activate_main_window(app_handle),
        _ => {}
    });
}

fn build_menu(app: &tauri::AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let settings = MenuItemBuilder::with_id(shell::MENU_SETTINGS, "Settings...")
        .accelerator("CmdOrCtrl+Comma")
        .build(app)?;
    let check_updates =
        MenuItemBuilder::with_id(shell::MENU_CHECK_FOR_UPDATES, "Check for Updates...")
            .build(app)?;
    let quit = MenuItemBuilder::with_id(shell::MENU_QUIT, "Quit Kandev")
        .accelerator("CmdOrCtrl+KeyQ")
        .build(app)?;
    let app_menu = build_application_menu(app, &settings, &check_updates, &quit)?;

    let new_task = MenuItemBuilder::with_id(shell::MENU_NEW_TASK, "New Task")
        .accelerator("CmdOrCtrl+KeyN")
        .build(app)?;
    let close_context = MenuItemBuilder::with_id(shell::MENU_CLOSE_CONTEXT, "Close Context")
        .accelerator("CmdOrCtrl+KeyW")
        .build(app)?;
    let file_menu = Submenu::with_items(app, "File", true, &[&new_task, &close_context])?;

    let edit_menu = Submenu::with_items(
        app,
        "Edit",
        true,
        &[
            &PredefinedMenuItem::undo(app, None)?,
            &PredefinedMenuItem::redo(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::cut(app, None)?,
            &PredefinedMenuItem::copy(app, None)?,
            &PredefinedMenuItem::paste(app, None)?,
            &PredefinedMenuItem::select_all(app, None)?,
        ],
    )?;

    let zoom_in = MenuItemBuilder::with_id(shell::MENU_ZOOM_IN, "Zoom In")
        .accelerator("CmdOrCtrl+Shift+Equal")
        .build(app)?;
    let zoom_in_equals = MenuItemBuilder::with_id(shell::MENU_ZOOM_IN_EQUALS, "Zoom In (=)")
        .accelerator("CmdOrCtrl+Equal")
        .build(app)?;
    let zoom_out = MenuItemBuilder::with_id(shell::MENU_ZOOM_OUT, "Zoom Out")
        .accelerator("CmdOrCtrl+Minus")
        .build(app)?;
    let actual_size = MenuItemBuilder::with_id(shell::MENU_ZOOM_RESET, "Actual Size")
        .accelerator("CmdOrCtrl+Digit0")
        .build(app)?;
    let fullscreen = MenuItemBuilder::with_id(shell::MENU_FULLSCREEN, "Toggle Full Screen")
        .accelerator(FULLSCREEN_ACCELERATOR)
        .build(app)?;
    let view_menu = Submenu::with_items(
        app,
        "View",
        true,
        &[
            &zoom_in,
            &zoom_in_equals,
            &zoom_out,
            &actual_size,
            &PredefinedMenuItem::separator(app)?,
            &fullscreen,
        ],
    )?;

    let window_menu = Submenu::with_items(
        app,
        "Window",
        true,
        &[
            &PredefinedMenuItem::minimize(app, None)?,
            &PredefinedMenuItem::maximize(app, None)?,
        ],
    )?;

    let docs =
        MenuItemBuilder::with_id(shell::MENU_HELP_DOCS, "Kandev Documentation").build(app)?;
    let repository =
        MenuItemBuilder::with_id(shell::MENU_HELP_REPOSITORY, "Kandev Repository").build(app)?;
    let releases =
        MenuItemBuilder::with_id(shell::MENU_HELP_RELEASES, "Release Notes").build(app)?;
    let help_menu = Submenu::with_items(app, "Help", true, &[&docs, &repository, &releases])?;

    Menu::with_items(
        app,
        &[
            &app_menu,
            &file_menu,
            &edit_menu,
            &view_menu,
            &window_menu,
            &help_menu,
        ],
    )
}

#[cfg(target_os = "macos")]
fn build_application_menu(
    app: &tauri::AppHandle,
    settings: &tauri::menu::MenuItem<tauri::Wry>,
    check_updates: &tauri::menu::MenuItem<tauri::Wry>,
    quit: &tauri::menu::MenuItem<tauri::Wry>,
) -> tauri::Result<Submenu<tauri::Wry>> {
    Submenu::with_items(
        app,
        "Kandev",
        true,
        &[
            &PredefinedMenuItem::about(app, None, None)?,
            &PredefinedMenuItem::separator(app)?,
            settings,
            check_updates,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::services(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::hide(app, None)?,
            &PredefinedMenuItem::hide_others(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            quit,
        ],
    )
}

#[cfg(not(target_os = "macos"))]
fn build_application_menu(
    app: &tauri::AppHandle,
    settings: &tauri::menu::MenuItem<tauri::Wry>,
    check_updates: &tauri::menu::MenuItem<tauri::Wry>,
    quit: &tauri::menu::MenuItem<tauri::Wry>,
) -> tauri::Result<Submenu<tauri::Wry>> {
    Submenu::with_items(
        app,
        "Kandev",
        true,
        &[
            &PredefinedMenuItem::about(app, None, None)?,
            &PredefinedMenuItem::separator(app)?,
            settings,
            check_updates,
            &PredefinedMenuItem::separator(app)?,
            quit,
        ],
    )
}

fn handle_menu_event(app: &tauri::AppHandle, event: tauri::menu::MenuEvent) {
    let Some(action) = shell::menu_action(event.id().as_ref()) else {
        return;
    };
    match action {
        MenuAction::Emit(event_name) => {
            focus_main_window(app);
            if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
                let _ = window.emit(event_name, ());
            }
        }
        MenuAction::ZoomIn | MenuAction::ZoomOut | MenuAction::ZoomReset => {
            apply_zoom(app, action);
        }
        MenuAction::Fullscreen => {
            if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
                if let Ok(fullscreen) = window.is_fullscreen() {
                    let _ = window.set_fullscreen(!fullscreen);
                }
            }
        }
        MenuAction::Quit => shutdown_and_exit(app),
        MenuAction::HelpDocs => open_help_url(app, "https://github.com/kdlbs/kandev#readme"),
        MenuAction::HelpRepository => open_help_url(app, "https://github.com/kdlbs/kandev"),
        MenuAction::HelpReleases => open_help_url(app, "https://github.com/kdlbs/kandev/releases"),
    }
}

fn open_help_url(app: &tauri::AppHandle, url: &str) {
    if let Err(error) = external_links::open_validated_external_url(app, url) {
        eprintln!("Could not open desktop help URL: {error}");
    }
}

fn apply_zoom(app: &tauri::AppHandle, action: MenuAction) {
    let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) else {
        return;
    };
    let zoom = app.state::<ZoomState>();
    let Some(level) = zoom.preview(action) else {
        return;
    };
    if window.set_zoom(level).is_ok() {
        zoom.commit(level);
    }
}

fn focus_main_window(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn activate_main_window(app: &tauri::AppHandle) {
    focus_main_window(app);
}

fn shutdown_and_exit(app: &tauri::AppHandle) {
    let state = app.state::<backend::BackendState>().inner().clone();
    if !state.begin_shutdown() {
        return;
    }
    let app = app.clone();
    thread::spawn(move || {
        state.stop();
        app.exit(0);
    });
}
