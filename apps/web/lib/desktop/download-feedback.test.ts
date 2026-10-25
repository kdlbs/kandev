import { afterEach, describe, expect, it } from "vitest";
import type { DesktopDownloadFeedback } from "./protocol";
import { desktopDownloadToastStatus } from "./download-feedback";

const event = (status: DesktopDownloadFeedback["status"], fileName: string) => ({
  status,
  fileName,
  url: "http://127.0.0.1:38430/export",
});

afterEach(() => {
  window.history.replaceState({}, "", "/");
});

describe("desktop download feedback", () => {
  it("shows a global result for non-Logs exports", () => {
    expect(desktopDownloadToastStatus(event("saved", "org-chart.svg"))).toBe("saved");
    expect(desktopDownloadToastStatus(event("failed", "agent-memory.json"))).toBe("failed");
  });

  it("leaves cancellation neutral and routes Logs results to their current surface", () => {
    window.history.replaceState({}, "", "/settings/system/data-storage?tab=logs");
    expect(desktopDownloadToastStatus(event("cancelled", "org-chart.svg"))).toBeNull();
    expect(desktopDownloadToastStatus(event("started", "org-chart.svg"))).toBeNull();
    expect(
      desktopDownloadToastStatus({
        ...event("saved", "my-logs.zip"),
        url: "http://127.0.0.1:38430/api/v1/system/logs/bundles/job-1/download",
      }),
    ).toBeNull();
  });

  it("does not suppress an unrelated export with the diagnostic bundle filename", () => {
    expect(desktopDownloadToastStatus(event("saved", "kandev-diagnostic-logs.zip"))).toBe("saved");
    expect(desktopDownloadToastStatus(event("failed", "kandev-diagnostic-logs.zip"))).toBe(
      "failed",
    );
  });

  it("suppresses duplicate feedback when a Logs export is renamed in the Save dialog", () => {
    window.history.replaceState({}, "", "/settings/system/data-storage?tab=logs");
    expect(
      desktopDownloadToastStatus({
        ...event("saved", "my-private-logs.zip"),
        url: "http://127.0.0.1:38430/api/v1/system/logs/bundles/job-1/download?token=secret",
      }),
    ).toBeNull();
  });

  it("shows the global result if the user leaves Logs before the download finishes", () => {
    const feedback = {
      ...event("saved", "my-private-logs.zip"),
      url: "http://127.0.0.1:38430/api/v1/system/logs/bundles/job-1/download",
    };

    window.history.pushState({}, "", "/settings/system/data-storage?tab=logs");
    expect(desktopDownloadToastStatus(feedback)).toBeNull();

    window.history.pushState({}, "", "/settings");
    expect(desktopDownloadToastStatus(feedback)).toBe("saved");
  });
});
