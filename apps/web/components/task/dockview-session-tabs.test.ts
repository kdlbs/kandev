import { describe, it, expect, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { reconcileRemovedSessionPanels } from "./dockview-session-tabs";

type FakePanel = {
  id: string;
  api: { close: ReturnType<typeof vi.fn> };
};

const KEEP = "keep";
const KEEP_PANEL = `session:${KEEP}`;
const LEAKED_PANEL = "session:leaked";

function makeApi(panelIds: string[]): { api: DockviewApi; panels: FakePanel[] } {
  const panels: FakePanel[] = panelIds.map((id) => ({
    id,
    api: { close: vi.fn() },
  }));
  const api = {
    panels,
    getPanel: (id: string) => panels.find((p) => p.id === id) ?? null,
  } as unknown as DockviewApi;
  return { api, panels };
}

describe("reconcileRemovedSessionPanels", () => {
  it("closes a stale tracked panel that's still live in dockview", () => {
    // createdSet has session-A; A's panel is live; A is no longer in the task's
    // session list — must be closed.
    const { api, panels } = makeApi(["session:A", KEEP_PANEL]);
    const createdSet = new Set(["A", KEEP]);

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

    const aPanel = panels.find((p) => p.id === "session:A");
    expect(aPanel?.api.close).toHaveBeenCalledTimes(1);
    expect(createdSet.has("A")).toBe(false);
  });

  it("closes live session panels that were never tracked in createdSet (the leak)", () => {
    // Reproduces the user-reported leak: dockview has session panels
    // (e.g. restored from a persisted layout) for sessions that aren't in the
    // current task's session list. createdSet is empty (or missing them)
    // because the panels entered via `tryRestoreLayout` / `fromJSON`, not
    // through ensureSessionPanel.
    const { api, panels } = makeApi([LEAKED_PANEL, KEEP_PANEL]);
    const createdSet = new Set<string>(["stale-deleted"]); // pollution from a prior right-click delete

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

    const leakedPanel = panels.find((p) => p.id === LEAKED_PANEL);
    const keepPanel = panels.find((p) => p.id === KEEP_PANEL);
    expect(
      leakedPanel?.api.close,
      "leaked session panel must be closed even though it was never tracked",
    ).toHaveBeenCalledTimes(1);
    expect(keepPanel?.api.close, "keepSessionId panel must not be closed").not.toHaveBeenCalled();
  });

  it("does not close the keepSessionId panel even if it is missing from createdSet", () => {
    const { api, panels } = makeApi([KEEP_PANEL]);
    const createdSet = new Set<string>(); // keep was never tracked

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

    const keepPanel = panels.find((p) => p.id === KEEP_PANEL);
    expect(keepPanel?.api.close).not.toHaveBeenCalled();
  });

  it("does not close panels for sessions still present in the task's session list", () => {
    const { api, panels } = makeApi(["session:a", "session:b"]);
    const createdSet = new Set<string>();

    reconcileRemovedSessionPanels(api, createdSet, ["a", "b"], "a");

    expect(panels.find((p) => p.id === "session:a")?.api.close).not.toHaveBeenCalled();
    expect(panels.find((p) => p.id === "session:b")?.api.close).not.toHaveBeenCalled();
  });

  it("ignores non-session panels", () => {
    const { api, panels } = makeApi(["sidebar", "terminal:1", LEAKED_PANEL]);
    const createdSet = new Set<string>();

    reconcileRemovedSessionPanels(api, createdSet, [], "");

    expect(panels.find((p) => p.id === "sidebar")?.api.close).not.toHaveBeenCalled();
    expect(panels.find((p) => p.id === "terminal:1")?.api.close).not.toHaveBeenCalled();
    expect(panels.find((p) => p.id === LEAKED_PANEL)?.api.close).toHaveBeenCalledTimes(1);
  });

  it("prunes stale entries from createdSet whose panels are already gone", () => {
    // Right-click delete path: the panel was removed via
    // containerApi.removePanel() in onDeleted, but createdSet still holds the
    // session ID. Reconcile should drop the stale entry.
    const { api } = makeApi([KEEP_PANEL]);
    const createdSet = new Set<string>(["already-removed", KEEP]);

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

    expect(createdSet.has("already-removed")).toBe(false);
    expect(createdSet.has(KEEP)).toBe(true);
  });
});
