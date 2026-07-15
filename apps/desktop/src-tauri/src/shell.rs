use std::sync::Mutex;

pub const DESKTOP_EVENT_PREFIX: &str = "kandev-desktop-v1-";
pub const CLOSE_CONTEXT_EVENT: &str = "kandev-desktop-v1-close-context";
pub const OPEN_SETTINGS_EVENT: &str = "kandev-desktop-v1-open-settings";
pub const NEW_TASK_EVENT: &str = "kandev-desktop-v1-new-task";
pub const CHECK_FOR_UPDATES_EVENT: &str = "kandev-desktop-v1-check-for-updates";

pub const MENU_SETTINGS: &str = "desktop.v1.open-settings";
pub const MENU_NEW_TASK: &str = "desktop.v1.new-task";
pub const MENU_CLOSE_CONTEXT: &str = "desktop.v1.close-context";
pub const MENU_CHECK_FOR_UPDATES: &str = "desktop.v1.check-for-updates";
pub const MENU_ZOOM_IN: &str = "desktop.v1.zoom-in";
pub const MENU_ZOOM_IN_EQUALS: &str = "desktop.v1.zoom-in-equals";
pub const MENU_ZOOM_OUT: &str = "desktop.v1.zoom-out";
pub const MENU_ZOOM_RESET: &str = "desktop.v1.zoom-reset";
pub const MENU_FULLSCREEN: &str = "desktop.v1.fullscreen";
pub const MENU_QUIT: &str = "desktop.v1.quit";
pub const MENU_HELP_DOCS: &str = "desktop.v1.help-docs";
pub const MENU_HELP_REPOSITORY: &str = "desktop.v1.help-repository";
pub const MENU_HELP_RELEASES: &str = "desktop.v1.help-releases";

const DEFAULT_ZOOM: f64 = 1.0;
const ZOOM_STEP: f64 = 0.1;
const MIN_ZOOM: f64 = 0.5;
const MAX_ZOOM: f64 = 2.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MenuAction {
    Emit(&'static str),
    ZoomIn,
    ZoomOut,
    ZoomReset,
    Fullscreen,
    Quit,
    HelpDocs,
    HelpRepository,
    HelpReleases,
}

pub fn menu_action(id: &str) -> Option<MenuAction> {
    match id {
        MENU_SETTINGS => Some(MenuAction::Emit(OPEN_SETTINGS_EVENT)),
        MENU_NEW_TASK => Some(MenuAction::Emit(NEW_TASK_EVENT)),
        MENU_CLOSE_CONTEXT => Some(MenuAction::Emit(CLOSE_CONTEXT_EVENT)),
        MENU_CHECK_FOR_UPDATES => Some(MenuAction::Emit(CHECK_FOR_UPDATES_EVENT)),
        MENU_ZOOM_IN | MENU_ZOOM_IN_EQUALS => Some(MenuAction::ZoomIn),
        MENU_ZOOM_OUT => Some(MenuAction::ZoomOut),
        MENU_ZOOM_RESET => Some(MenuAction::ZoomReset),
        MENU_FULLSCREEN => Some(MenuAction::Fullscreen),
        MENU_QUIT => Some(MenuAction::Quit),
        MENU_HELP_DOCS => Some(MenuAction::HelpDocs),
        MENU_HELP_REPOSITORY => Some(MenuAction::HelpRepository),
        MENU_HELP_RELEASES => Some(MenuAction::HelpReleases),
        _ => None,
    }
}

#[derive(Debug)]
pub struct ZoomState {
    level: Mutex<f64>,
}

impl Default for ZoomState {
    fn default() -> Self {
        Self {
            level: Mutex::new(DEFAULT_ZOOM),
        }
    }
}

impl ZoomState {
    pub fn current(&self) -> f64 {
        *self.level.lock().expect("zoom state mutex poisoned")
    }

    pub fn preview(&self, action: MenuAction) -> Option<f64> {
        let current = self.current();
        match action {
            MenuAction::ZoomIn => Some(clamp_zoom(current + ZOOM_STEP)),
            MenuAction::ZoomOut => Some(clamp_zoom(current - ZOOM_STEP)),
            MenuAction::ZoomReset => Some(DEFAULT_ZOOM),
            _ => None,
        }
    }

    pub fn commit(&self, level: f64) {
        *self.level.lock().expect("zoom state mutex poisoned") = clamp_zoom(level);
    }
}

fn clamp_zoom(level: f64) -> f64 {
    level.clamp(MIN_ZOOM, MAX_ZOOM)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ShutdownIntent {
    ContextClose,
    WindowClose,
    Quit,
    UpdaterRestart,
}

pub fn should_stop_backend(intent: ShutdownIntent) -> bool {
    !matches!(intent, ShutdownIntent::ContextClose)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn contextual_close_maps_only_to_the_versioned_bridge_event() {
        assert_eq!(
            menu_action(MENU_CLOSE_CONTEXT),
            Some(MenuAction::Emit(CLOSE_CONTEXT_EVENT))
        );
        assert!(CLOSE_CONTEXT_EVENT.starts_with(DESKTOP_EVENT_PREFIX));
        assert!(!should_stop_backend(ShutdownIntent::ContextClose));
    }

    #[test]
    fn native_commands_map_to_frozen_v1_events() {
        assert_eq!(CLOSE_CONTEXT_EVENT, "kandev-desktop-v1-close-context");
        assert_eq!(OPEN_SETTINGS_EVENT, "kandev-desktop-v1-open-settings");
        assert_eq!(NEW_TASK_EVENT, "kandev-desktop-v1-new-task");
        assert_eq!(
            CHECK_FOR_UPDATES_EVENT,
            "kandev-desktop-v1-check-for-updates"
        );
        assert_eq!(
            menu_action(MENU_SETTINGS),
            Some(MenuAction::Emit(OPEN_SETTINGS_EVENT))
        );
        assert_eq!(
            menu_action(MENU_NEW_TASK),
            Some(MenuAction::Emit(NEW_TASK_EVENT))
        );
        assert_eq!(
            menu_action(MENU_CHECK_FOR_UPDATES),
            Some(MenuAction::Emit(CHECK_FOR_UPDATES_EVENT))
        );
    }

    #[test]
    fn zoom_clamps_and_reset_restores_actual_size() {
        let state = ZoomState::default();

        for _ in 0..30 {
            let level = state.preview(MenuAction::ZoomIn).unwrap();
            state.commit(level);
        }
        assert_eq!(state.current(), MAX_ZOOM);

        for _ in 0..30 {
            let level = state.preview(MenuAction::ZoomOut).unwrap();
            state.commit(level);
        }
        assert_eq!(state.current(), MIN_ZOOM);

        state.commit(state.preview(MenuAction::ZoomReset).unwrap());
        assert_eq!(state.current(), DEFAULT_ZOOM);
    }

    #[test]
    fn failed_zoom_can_leave_the_previous_level_uncommitted() {
        let state = ZoomState::default();
        assert_eq!(state.preview(MenuAction::ZoomIn), Some(1.1));
        assert_eq!(state.current(), DEFAULT_ZOOM);
    }

    #[test]
    fn close_quit_and_updater_restart_stop_the_backend() {
        assert!(should_stop_backend(ShutdownIntent::WindowClose));
        assert!(should_stop_backend(ShutdownIntent::Quit));
        assert!(should_stop_backend(ShutdownIntent::UpdaterRestart));
    }
}
