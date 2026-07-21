export const DESKTOP_PROTOCOL_VERSION = "v1" as const;
const desktopEvent = <TName extends string>(
  name: TName,
): `kandev-desktop-${typeof DESKTOP_PROTOCOL_VERSION}-${TName}` =>
  `kandev-desktop-${DESKTOP_PROTOCOL_VERSION}-${name}`;

export const DESKTOP_NATIVE_EVENTS = {
  "close-context": desktopEvent("close-context"),
  "open-settings": desktopEvent("open-settings"),
  "new-task": desktopEvent("new-task"),
  "check-for-updates": desktopEvent("check-for-updates"),
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
