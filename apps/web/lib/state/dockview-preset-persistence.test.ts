import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";
import { persistEnvLayoutNow } from "./dockview-store";

vi.mock("@/lib/local-storage", () => ({
  setEnvLayout: vi.fn(),
  getEnvLayout: vi.fn(() => null),
  getEnvMaximizeState: vi.fn(() => null),
  setEnvMaximizeState: vi.fn(),
  removeEnvMaximizeState: vi.fn(),
  getGlobalSidebarWidth: vi.fn(() => null),
  setGlobalSidebarWidth: vi.fn(),
  clearGlobalSidebarWidth: vi.fn(),
}));

import { setEnvLayout } from "@/lib/local-storage";

function makeApi(snapshot: object = { columns: [] }): DockviewApi {
  return {
    toJSON: vi.fn(() => snapshot),
  } as unknown as DockviewApi;
}

// Regression: applyBuiltInPreset and applyCustomLayout used to mutate the live
// layout in memory but never wrote it to env-keyed sessionStorage. The auto-save
// in setupLayoutPersistence is gated by isRestoringLayout=true (which both
// actions hold for the entire rAF window in which they emit layout-change
// events), so the debounced save never fired. A page refresh after a preset
// switch restored the pre-preset layout from sessionStorage, losing the user's
// intent. persistEnvLayoutNow runs after isRestoringLayout flips back to false
// at the end of those rAF callbacks.
describe("persistEnvLayoutNow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("writes the live api.toJSON() to env storage when envId is set", () => {
    const snapshot = { columns: [{ id: "center" }] };
    const api = makeApi(snapshot);

    persistEnvLayoutNow(api, "env-1", null);

    expect(setEnvLayout).toHaveBeenCalledTimes(1);
    expect(setEnvLayout).toHaveBeenCalledWith("env-1", snapshot);
  });

  it("is a no-op when envId is null (no env adopted yet)", () => {
    const api = makeApi();

    persistEnvLayoutNow(api, null, null);

    expect(setEnvLayout).not.toHaveBeenCalled();
    expect(api.toJSON).not.toHaveBeenCalled();
  });

  it("is a no-op while maximized (toJSON would be the 2-column overlay)", () => {
    // saveOutgoingEnv owns the maximize slot. Writing the overlay as the env's
    // regular layout would resurface a truncated layout on reload.
    const api = makeApi();
    const preMaximizeLayout = { columns: [] };

    persistEnvLayoutNow(api, "env-1", preMaximizeLayout);

    expect(setEnvLayout).not.toHaveBeenCalled();
    expect(api.toJSON).not.toHaveBeenCalled();
  });

  it("swallows serialization/storage errors so callers do not crash mid-rAF", () => {
    const api = {
      toJSON: vi.fn(() => {
        throw new Error("dockview serialize failed");
      }),
    } as unknown as DockviewApi;

    expect(() => persistEnvLayoutNow(api, "env-1", null)).not.toThrow();
    expect(setEnvLayout).not.toHaveBeenCalled();
  });
});
