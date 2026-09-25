import type { DesktopDownloadFeedback } from "./protocol";

export const DIAGNOSTIC_BUNDLE_FILE_NAME = "kandev-diagnostic-logs.zip";

export function desktopDownloadToastStatus(
  feedback: DesktopDownloadFeedback,
): "saved" | "failed" | null {
  if (isDiagnosticBundleDownload(feedback.url)) return null;
  if (feedback.status === "saved" || feedback.status === "failed") return feedback.status;
  return null;
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
