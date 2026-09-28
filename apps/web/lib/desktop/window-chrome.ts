type TauriWindow = Window & { __TAURI_INTERNALS__?: unknown };

type MacTauriDragRegionProps = { "data-tauri-drag-region"?: "deep" };

function browserWindow(): Window | undefined {
  return typeof window === "undefined" ? undefined : window;
}

function browserUserAgent(): string {
  return typeof navigator === "undefined" ? "" : navigator.userAgent;
}

export function isMacTauriWebview(
  win: Window | undefined = browserWindow(),
  userAgent = browserUserAgent(),
): boolean {
  return Boolean(
    (win as TauriWindow | undefined)?.__TAURI_INTERNALS__ && /Macintosh|Mac OS X/i.test(userAgent),
  );
}

export function macTauriDragRegionProps(win?: Window, userAgent?: string): MacTauriDragRegionProps {
  return isMacTauriWebview(win, userAgent) ? { "data-tauri-drag-region": "deep" } : {};
}
