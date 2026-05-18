import { describe, it, expect, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { reconcileRemovedSessionPanels } from "./dockview-session-tabs";

type FakePanel = {
  id: string;
  api: { close: ReturnType<typeof vi.fn<[], void>> };
};

const KEEP = "keep";
const KEEP_PANEL = `session:${KEEP}`;
const LEAKED_PANEL = "session:leaked";

/**
 * Builds a fake DockviewApi where `panel.api.close()` mutates the underlying
 * `panels` array synchronously, mirroring how dockview removes a panel from
 * its live `panels` getter the moment it's closed. This is what made the
 * unsnapshotted `for (const panel of api.panels)` loop skip elements.
 */
function makeApi(panelIds: string[]): { api: DockviewApi; panels: FakePanel[] } {
  const panels: FakePanel[] = [];
  for (const id of panelIds) {
    const panel: FakePanel = {
      id,
      api: {
        close: vi.fn(() => {
          const idx = panels.indexOf(panel);
          if (idx !== -1) panels.splice(idx, 1);
        }),
      },
    };
    panels.push(panel);
  }
  const api = {
    panels,
    getPanel: (id: string) => panels.find((p) => p.id === id) ?? null,
  } as unknown as DockviewApi;
  return { api, panels };
}

function panelById(panels: FakePanel[], id: string): FakePanel | undefined {
  return panels.find((p) => p.id === id);
}

describe("reconcileRemovedSessionPanels", () => {
  it("closes a stale tracked panel that's still live in dockview", () => {
    // createdSet has session-A; A's panel is live; A is no longer in the task's
    // session list, so it must be closed.
    const { api, panels } = makeApi(["session:A", KEEP_PANEL]);
    const aPanel = panelById(panels, "session:A");
    const createdSet = new Set(["A", KEEP]);

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

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
    const leakedPanel = panelById(panels, LEAKED_PANEL);
    const keepPanel = panelById(panels, KEEP_PANEL);
    const createdSet = new Set<string>(["stale-deleted"]); // pollution from a prior right-click delete

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

    expect(
      leakedPanel?.api.close,
      "leaked session panel must be closed even though it was never tracked",
    ).toHaveBeenCalledTimes(1);
    expect(keepPanel?.api.close, "keepSessionId panel must not be closed").not.toHaveBeenCalled();
  });

  it("closes every leaked panel even when close() mutates api.panels mid-iteration", () => {
    // Regression for iterator-invalidation: with a live `panels` getter, a
    // synchronous splice inside `close()` shifts subsequent elements left and
    // a `for (const p of api.panels)` loop would skip the panel that moved
    // into the just-vacated index. The implementation snapshots before
    // iterating, so all leaked panels must still be closed in one pass.
    const { api, panels } = makeApi([
      "session:leak1",
      "session:leak2",
      "session:leak3",
      KEEP_PANEL,
    ]);
    const leak1 = panelById(panels, "session:leak1");
    const leak2 = panelById(panels, "session:leak2");
    const leak3 = panelById(panels, "session:leak3");
    const keepPanel = panelById(panels, KEEP_PANEL);

    reconcileRemovedSessionPanels(api, new Set<string>(), [KEEP], KEEP);

    expect(leak1?.api.close).toHaveBeenCalledTimes(1);
    expect(leak2?.api.close).toHaveBeenCalledTimes(1);
    expect(leak3?.api.close).toHaveBeenCalledTimes(1);
    expect(keepPanel?.api.close).not.toHaveBeenCalled();
  });

  it("does not close the keepSessionId panel even if it is missing from createdSet", () => {
    const { api, panels } = makeApi([KEEP_PANEL]);
    const keepPanel = panelById(panels, KEEP_PANEL);
    const createdSet = new Set<string>(); // keep was never tracked

    reconcileRemovedSessionPanels(api, createdSet, [KEEP], KEEP);

    expect(keepPanel?.api.close).not.toHaveBeenCalled();
  });

  it("does not close panels for sessions still present in the task's session list", () => {
    const { api, panels } = makeApi(["session:a", "session:b"]);
    const a = panelById(panels, "session:a");
    const b = panelById(panels, "session:b");
    const createdSet = new Set<string>();

    reconcileRemovedSessionPanels(api, createdSet, ["a", "b"], "a");

    expect(a?.api.close).not.toHaveBeenCalled();
    expect(b?.api.close).not.toHaveBeenCalled();
  });

  it("ignores non-session panels", () => {
    const { api, panels } = makeApi(["sidebar", "terminal:1", LEAKED_PANEL]);
    const sidebar = panelById(panels, "sidebar");
    const terminal = panelById(panels, "terminal:1");
    const leaked = panelById(panels, LEAKED_PANEL);
    const createdSet = new Set<string>();

    reconcileRemovedSessionPanels(api, createdSet, [], "");

    expect(sidebar?.api.close).not.toHaveBeenCalled();
    expect(terminal?.api.close).not.toHaveBeenCalled();
    expect(leaked?.api.close).toHaveBeenCalledTimes(1);
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
