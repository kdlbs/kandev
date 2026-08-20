export const STORAGE_KEYS = {
  BACKEND_URL: "kandev.settings.backendUrl",
  ONBOARDING_COMPLETED: "kandev.onboarding.completed",
  /** Last settings page opened on this device; where bare `/settings` resolves. */
  LAST_SETTINGS_PATH: "kandev.settings.lastPath",
  /** How the settings menu renders on this device: flat, accordion, persistent. */
  SETTINGS_MENU_MODE: "kandev.settings.menuMode",
  /** Branch keys left open in the persistent tree on this device. */
  SETTINGS_MENU_EXPANDED: "kandev.settings.menuExpanded",
  /** Whether agent rich-output line and bar charts animate on this device. */
  RICH_OUTPUT_ANIMATIONS: "kandev.settings.richOutputAnimations",
} as const;

export const DEFAULT_BACKEND_URL = "http://localhost:38429";

// Kanban Preview Panel Settings
export const PREVIEW_PANEL = {
  // Minimum width of the preview panel in pixels (prevents panel from being too narrow)
  MIN_WIDTH_PX: 300,

  // Default width of the preview panel in pixels when first opened
  DEFAULT_WIDTH_PX: 500,

  // Maximum width of the preview panel in viewport width percentage (prevents covering entire screen)
  MAX_WIDTH_VW: 95,

  // Minimum width of the kanban board as a percentage of viewport before the panel switches to floating mode
  MIN_KANBAN_WIDTH_PERCENT: 50,
} as const;
