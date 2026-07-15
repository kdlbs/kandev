export const DESKTOP_PROTOCOL_VERSION = "v1" as const;

export const DESKTOP_NATIVE_EVENTS = {
  "close-context": "kandev-desktop-v1-close-context",
  "open-settings": "kandev-desktop-v1-open-settings",
  "new-task": "kandev-desktop-v1-new-task",
  "check-for-updates": "kandev-desktop-v1-check-for-updates",
} as const;

export type DesktopEventName = keyof typeof DESKTOP_NATIVE_EVENTS;

export type DesktopEventPayloads = {
  "close-context": undefined;
  "open-settings": undefined;
  "new-task": undefined;
  "check-for-updates": undefined;
};

export const DESKTOP_NATIVE_COMMANDS = {
  "get-update-state": "get_update_state",
  "check-for-updates": "check_for_updates",
  "install-update": "install_update",
} as const;

export type DesktopUpdatePhase =
  | "idle"
  | "checking"
  | "available"
  | "up-to-date"
  | "downloading"
  | "installing"
  | "error";

export type DesktopUpdateState = {
  phase: DesktopUpdatePhase;
  currentVersion: string;
  latestVersion: string | null;
  releaseNotes: string | null;
  releaseUrl: string | null;
  checkedAtEpochMs: number | null;
  downloadedBytes: number | null;
  totalBytes: number | null;
  installSupported: boolean;
  installUnsupportedReason: string | null;
  error: string | null;
};
