import { describe, expect, it, vi } from "vitest";
import { createDesktopUpdaterAdapter, type DesktopInvokeTransport } from "./updater-adapter";

const state = {
  phase: "up-to-date" as const,
  currentVersion: "1.0.0",
  latestVersion: null,
  releaseNotes: null,
  releaseUrl: null,
  checkedAtEpochMs: 42,
  downloadedBytes: null,
  totalBytes: null,
  installSupported: true,
  installUnsupportedReason: null,
  error: null,
};

describe("desktop updater adapter", () => {
  it("maps the frozen v1 command names to bounded native commands", async () => {
    const invoke = vi.fn().mockResolvedValue(state);
    const transport: DesktopInvokeTransport = { isAvailable: () => true, invoke };
    const adapter = createDesktopUpdaterAdapter(transport);

    await expect(adapter.getState()).resolves.toEqual(state);
    await expect(adapter.checkForUpdates()).resolves.toEqual(state);
    await expect(adapter.installUpdate()).resolves.toEqual(state);

    expect(invoke.mock.calls.map(([command]) => command)).toEqual([
      "get_update_state",
      "check_for_updates",
      "install_update",
    ]);
  });

  it("rejects updater commands when Tauri is absent", async () => {
    const invoke = vi.fn();
    const adapter = createDesktopUpdaterAdapter({ isAvailable: () => false, invoke });

    await expect(adapter.checkForUpdates()).rejects.toThrow("desktop updater is unavailable");
    expect(invoke).not.toHaveBeenCalled();
  });
});
