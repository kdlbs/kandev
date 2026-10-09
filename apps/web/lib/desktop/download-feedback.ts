import type { DesktopDownloadFeedback } from "./protocol";

export const DIAGNOSTIC_BUNDLE_FILE_NAME = "kandev-diagnostic-logs.zip";

export function desktopDownloadToastStatus(
  feedback: DesktopDownloadFeedback,
): "saved" | "failed" | null {
  if (isDiagnosticBundleDownload(feedback.url) && isLogsPage()) return null;
  if (feedback.status === "saved" || feedback.status === "failed") return feedback.status;
  return null;
}

function isLogsPage(): boolean {
  return (
    window.location.pathname === "/settings/system/data-storage" &&
    new URLSearchParams(window.location.search).get("tab") === "logs"
  );
}

function isDiagnosticBundleDownload(downloadUrl: string): boolean {
  try {
    return /\/api\/v1\/system\/logs\/bundles\/[^/]+\/download\/?$/.test(
      new URL(downloadUrl).pathname,
    );
  } catch {
    return false;
  }
}
