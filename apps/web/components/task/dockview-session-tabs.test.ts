import { describe, it, expect, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { TaskSession } from "@/lib/types/http";
import {
  ensureSessionTabPrecedesNonSessionTabs,
  findSessionAnchorGroupId,
  reconcileRemovedSessionPanels,
  resolveInitialPosition,
  resolveSessionTabSyncTarget,
  runAutoSessionTabEffect,
  shouldActivateSessionPanel,
  shouldRebuildDefaultForPendingSession,
} from "./dockview-session-tabs";
import { CENTER_GROUP, RIGHT_TOP_GROUP } from "@/lib/state/layout-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";

type FakePanel = {
  id: string;
  api: { close: ReturnType<typeof vi.fn<[], void>> };
};

type MoveToOptions = {
  group: unknown;
  position: "center";
  index: number;
  skipSetActive: boolean;
};

type MoveToCall = {
  panelId: string;
  options: MoveToOptions;
};

const KEEP = "keep";
const KEEP_PANEL = `session:${KEEP}`;
const LEAKED_PANEL = "session:leaked";
const PENDING_SESSION_ID = "session-new";
const TEST_GROUP_CENTER = "group-center";
const CENTER_POSITION = "center";
const SIBLING_PANEL = "session:sibling";

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

function makePositionApi(args: {
  groups: string[];
  panels?: Array<{ id: string; groupId: string }>;
}): DockviewApi {
  const panels =
    args.panels?.map((p) => ({
      id: p.id,
      group: { id: p.groupId },
    })) ?? [];
  return {
    groups: args.groups.map((id) => ({ id })),
    panels,
    getPanel: (id: string) => panels.find((p) => p.id === id) ?? null,
  } as unknown as DockviewApi;
}

function panelById(panels: FakePanel[], id: string): FakePanel | undefined {
  return panels.find((p) => p.id === id);
}

function makeTabOrderApi(panelIds: string[]): {
  api: DockviewApi;
  moveToCalls: MoveToCall[];
} {
  const moveToCalls: MoveToCall[] = [];
  const group = { id: TEST_GROUP_CENTER, panels: [] as Array<{ id: string }> };
  const panels = panelIds.map((id) => ({
    id,
    group,
    api: {
      moveTo: vi.fn((options: MoveToOptions) => {
        moveToCalls.push({ panelId: id, options });
      }),
    },
  }));
  group.panels = panels;
  return {
    api: {
      getPanel: (id: string) => panels.find((p) => p.id === id) ?? null,
    } as unknown as DockviewApi,
    moveToCalls,
  };
}

function expectedMove(panelId: string, index: number): MoveToCall {
  return {
    panelId,
    options: {
      group: expect.objectContaining({ id: TEST_GROUP_CENTER }),
      position: CENTER_POSITION,
      index,
      skipSetActive: true,
    },
  };
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

describe("findSessionAnchorGroupId", () => {
  function makeApiWithPanels(panels: Array<{ id: string; groupId: string }>): DockviewApi {
    const enriched = panels.map((p) => ({ id: p.id, group: { id: p.groupId } }));
    return {
      panels: enriched,
      getPanel: (id: string) => enriched.find((p) => p.id === id) ?? null,
    } as unknown as DockviewApi;
  }

  it("returns the group id of a pr-detail anchor panel", () => {
    // Regression: when a saved layout's session was sanitized away (page load)
    // or replaced (env switch) but pr-detail remained, the new session would
    // be added as a right-of-sidebar split instead of joining pr-detail's
    // group — pulling pr-detail out of the user's grouping with the agent.
    const api = makeApiWithPanels([{ id: "pr-detail", groupId: "saved-center-group" }]);

    expect(findSessionAnchorGroupId(api)).toBe("saved-center-group");
  });

  it("matches keyed pr-detail panels used by multi-repo PR flows", () => {
    // Multi-repo tasks open one PR tab per PR with id `pr-detail|owner/repo/N`
    // (see addPRPanel in dockview-panel-actions.ts). Without prefix matching,
    // findSessionAnchorGroupId would miss them and the new session would land
    // as a right-of-sidebar split — losing the user's PR/session grouping.
    const api = makeApiWithPanels([{ id: "pr-detail|owner/repo/123", groupId: "keyed-pr-group" }]);

    expect(findSessionAnchorGroupId(api)).toBe("keyed-pr-group");
  });

  it("returns null when no anchor panel exists", () => {
    const api = makeApiWithPanels([]);

    expect(findSessionAnchorGroupId(api)).toBeNull();
  });
});

describe("resolveInitialPosition", () => {
  it("creates a center column left of the right sidebar when only the right group remains", () => {
    useDockviewStore.setState({ centerGroupId: RIGHT_TOP_GROUP });
    const api = makePositionApi({ groups: [RIGHT_TOP_GROUP] });

    expect(resolveInitialPosition(api)).toEqual({
      referenceGroup: RIGHT_TOP_GROUP,
      direction: "left",
    });
  });
});

describe("ensureSessionTabPrecedesNonSessionTabs", () => {
  it("moves a restored session tab before a PR tab that was saved first", () => {
    const { api, moveToCalls } = makeTabOrderApi(["pr-detail", KEEP_PANEL, "preview:file-diff"]);

    ensureSessionTabPrecedesNonSessionTabs(api, KEEP);

    expect(moveToCalls).toEqual([expectedMove(KEEP_PANEL, 0)]);
  });

  it("moves interleaved session tabs as a stable block before non-session tabs", () => {
    const { api, moveToCalls } = makeTabOrderApi(["pr-detail", SIBLING_PANEL, KEEP_PANEL]);

    ensureSessionTabPrecedesNonSessionTabs(api, KEEP);

    expect(moveToCalls).toEqual([expectedMove(KEEP_PANEL, 0), expectedMove(SIBLING_PANEL, 0)]);
  });

  it("does not move when session tabs already precede non-session tabs", () => {
    const { api, moveToCalls } = makeTabOrderApi([KEEP_PANEL, SIBLING_PANEL, "pr-detail"]);

    ensureSessionTabPrecedesNonSessionTabs(api, KEEP);

    expect(moveToCalls).toEqual([]);
  });

  it("does not move when the session tab already precedes non-session tabs", () => {
    const { api, moveToCalls } = makeTabOrderApi([KEEP_PANEL, "pr-detail"]);

    ensureSessionTabPrecedesNonSessionTabs(api, KEEP);

    expect(moveToCalls).toEqual([]);
  });

  it("keeps earlier session tabs ahead of the active session tab", () => {
    const { api, moveToCalls } = makeTabOrderApi(["session:older", "pr-detail", KEEP_PANEL]);

    ensureSessionTabPrecedesNonSessionTabs(api, KEEP);

    expect(moveToCalls).toEqual([expectedMove(KEEP_PANEL, 1)]);
  });

  it("does not move when the target session panel is absent", () => {
    const { api, moveToCalls } = makeTabOrderApi(["pr-detail", SIBLING_PANEL]);

    ensureSessionTabPrecedesNonSessionTabs(api, KEEP);

    expect(moveToCalls).toEqual([]);
  });
});

describe("shouldRebuildDefaultForPendingSession", () => {
  it("does not rebuild when there is no effective session id", () => {
    const api = makePositionApi({
      groups: [RIGHT_TOP_GROUP],
      panels: [
        { id: "session:missing", groupId: RIGHT_TOP_GROUP },
        { id: "files", groupId: RIGHT_TOP_GROUP },
      ],
    });

    expect(shouldRebuildDefaultForPendingSession(api, null, [])).toBe(false);
  });

  it("rebuilds default when the active session is pending and no chat group remains", () => {
    const api = makePositionApi({
      groups: [RIGHT_TOP_GROUP],
      panels: [
        { id: "files", groupId: RIGHT_TOP_GROUP },
        { id: "changes", groupId: RIGHT_TOP_GROUP },
      ],
    });

    expect(shouldRebuildDefaultForPendingSession(api, PENDING_SESSION_ID, [])).toBe(true);
  });

  it("rebuilds before stale session panels are reconciled away", () => {
    const api = makePositionApi({
      groups: ["stale-center", RIGHT_TOP_GROUP],
      panels: [
        { id: "session:old", groupId: "stale-center" },
        { id: "files", groupId: RIGHT_TOP_GROUP },
        { id: "changes", groupId: RIGHT_TOP_GROUP },
      ],
    });

    expect(shouldRebuildDefaultForPendingSession(api, PENDING_SESSION_ID, [])).toBe(true);
  });

  it("does not rebuild while the chat placeholder is still present", () => {
    const api = makePositionApi({
      groups: [CENTER_GROUP, RIGHT_TOP_GROUP],
      panels: [
        { id: "chat", groupId: CENTER_GROUP },
        { id: "files", groupId: RIGHT_TOP_GROUP },
      ],
    });

    expect(shouldRebuildDefaultForPendingSession(api, PENDING_SESSION_ID, [])).toBe(false);
  });

  it("does not rebuild once the active session is hydrated for the task", () => {
    const api = makePositionApi({ groups: [RIGHT_TOP_GROUP] });

    expect(
      shouldRebuildDefaultForPendingSession(api, PENDING_SESSION_ID, [PENDING_SESSION_ID]),
    ).toBe(false);
  });
});

describe("resolveSessionTabSyncTarget", () => {
  function makeSession(id: string, taskId: string): TaskSession {
    return { id, task_id: taskId } as unknown as TaskSession;
  }

  it("returns the (activeTaskId, sid) pair for a session that belongs to the active task", () => {
    const target = resolveSessionTabSyncTarget({
      panelId: "session:s-new",
      activeTaskId: "task-A",
      activeSessionId: "s-old",
      taskSessionsById: { "s-new": makeSession("s-new", "task-A") },
      environmentIdBySessionId: { "s-new": "env-A" },
    });

    expect(target).toEqual({ taskId: "task-A", sessionId: "s-new" });
  });

  it("returns null for non-session panels (sidebar, files, terminal, ...)", () => {
    for (const panelId of ["sidebar", "files", "changes", "terminal:default", "pr-detail"]) {
      expect(
        resolveSessionTabSyncTarget({
          panelId,
          activeTaskId: "task-A",
          activeSessionId: null,
          taskSessionsById: {},
          environmentIdBySessionId: {},
        }),
      ).toBeNull();
    }
  });

  it("returns null when the activated session matches activeSessionId (no-op)", () => {
    const target = resolveSessionTabSyncTarget({
      panelId: "session:s-current",
      activeTaskId: "task-A",
      activeSessionId: "s-current",
      taskSessionsById: { "s-current": makeSession("s-current", "task-A") },
      environmentIdBySessionId: { "s-current": "env-A" },
    });

    expect(target).toBeNull();
  });

  /**
   * Regression for a layout-leak observed when creating a new task: during
   * the task switch dockview can briefly fire `onDidActivePanelChange` for a
   * stale `session:<oldSid>` panel that still belongs to the previous task.
   * If we wrote `setActiveSession(newTaskId, oldSid)`, that would poison
   * `lastSessionByTaskId[newTaskId]` and the next re-entry to the new task
   * would resolve to the cross-task session, restoring the wrong layout
   * (e.g. the previous task's files/changes panels).
   */
  it("returns null when the activated session belongs to a different task than activeTaskId", () => {
    const target = resolveSessionTabSyncTarget({
      panelId: "session:s-belongs-to-B",
      activeTaskId: "task-A",
      activeSessionId: null,
      taskSessionsById: { "s-belongs-to-B": makeSession("s-belongs-to-B", "task-B") },
      environmentIdBySessionId: { "s-belongs-to-B": "env-B" },
    });

    expect(target).toBeNull();
  });

  it("accepts a session that has an env mapping but isn't yet hydrated into taskSessions.items", () => {
    // The listener can fire before `session.state_changed` lands the new
    // session in the items map. The env mapping is what guarantees the session
    // exists at all — once that's present we trust the active task.
    const target = resolveSessionTabSyncTarget({
      panelId: "session:s-pending-hydration",
      activeTaskId: "task-A",
      activeSessionId: null,
      taskSessionsById: {},
      environmentIdBySessionId: { "s-pending-hydration": "env-A" },
    });

    expect(target).toEqual({ taskId: "task-A", sessionId: "s-pending-hydration" });
  });

  /**
   * Regression for a one-frame UI glitch: `removeTaskSession` clears
   * `taskSessions.items[sid]` and `environmentIdBySessionId[sid]` atomically.
   * A dying panel firing activation between the clear and unmount must not
   * write `activeSessionId = <deletedSid>` — the env-mapping gate catches it.
   */
  it("returns null when the activated session has no env mapping (deleted or stale)", () => {
    const target = resolveSessionTabSyncTarget({
      panelId: "session:s-deleted",
      activeTaskId: "task-A",
      activeSessionId: null,
      taskSessionsById: {},
      environmentIdBySessionId: {},
    });

    expect(target).toBeNull();
  });

  it("returns null when there is no active task", () => {
    const target = resolveSessionTabSyncTarget({
      panelId: "session:s",
      activeTaskId: null,
      activeSessionId: null,
      taskSessionsById: {},
      environmentIdBySessionId: { s: "env-A" },
    });

    expect(target).toBeNull();
  });

  it("returns null for a malformed session:<empty> panel id", () => {
    const target = resolveSessionTabSyncTarget({
      panelId: "session:",
      activeTaskId: "task-A",
      activeSessionId: null,
      taskSessionsById: {},
      environmentIdBySessionId: {},
    });

    expect(target).toBeNull();
  });
});

describe("shouldActivateSessionPanel", () => {
  const SID = "s-current";
  const SESSION_PANEL_ID = `session:${SID}`;
  const baseArgs = {
    prevTaskId: null,
    prevSessionId: null,
    currentTaskId: "task-A",
    currentSessionId: SID,
    currentActivePanelId: null,
  };

  it("activates when the session panel did not exist before (fresh creation)", () => {
    expect(shouldActivateSessionPanel({ ...baseArgs, sessionPanelExistedBefore: false })).toBe(
      true,
    );
  });

  it("activates on first mount when no panel is currently active", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        currentActivePanelId: null,
      }),
    ).toBe(true);
  });

  it("activates on first mount when the restored active panel is already the session", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        currentActivePanelId: SESSION_PANEL_ID,
      }),
    ).toBe(true);
  });

  /**
   * Regression: on page refresh, dockview's fromJSON restores the active
   * panel from the saved layout (e.g. a file diff the user had focused).
   * We must not force-activate the session panel on top of it.
   */
  it("does NOT override a restored non-session active panel on first mount", () => {
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        currentActivePanelId: "preview:commit-detail",
      }),
    ).toBe(false);
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        currentActivePanelId: "preview:file-editor",
      }),
    ).toBe(false);
    expect(
      shouldActivateSessionPanel({
        ...baseArgs,
        sessionPanelExistedBefore: true,
        currentActivePanelId: "plan",
      }),
    ).toBe(false);
  });

  it("activates on intra-task session switch (same task, different session)", () => {
    expect(
      shouldActivateSessionPanel({
        sessionPanelExistedBefore: true,
        prevTaskId: "task-A",
        prevSessionId: "s-old",
        currentTaskId: "task-A",
        currentSessionId: SID,
        currentActivePanelId: "preview:commit-detail",
      }),
    ).toBe(true);
  });

  it("does NOT activate on task switch (preserves saved active panel for new task)", () => {
    expect(
      shouldActivateSessionPanel({
        sessionPanelExistedBefore: true,
        prevTaskId: "task-A",
        prevSessionId: "s-old",
        currentTaskId: "task-B",
        currentSessionId: SID,
        currentActivePanelId: "preview:commit-detail",
      }),
    ).toBe(false);
  });
});

describe("runAutoSessionTabEffect", () => {
  it("updates previous refs when panel ensure is skipped for an unhydrated session", () => {
    const api = {
      panels: [{ id: "chat" }],
      getPanel: (id: string) => (id === "chat" ? { id: "chat" } : null),
    } as unknown as DockviewApi;
    const appStore = {
      getState: () => ({
        tasks: { activeTaskId: "task-A" },
        taskSessionsByTask: { itemsByTaskId: { "task-A": [] } },
      }),
    };
    const refs = {
      sessionTabCreatedRef: { current: new Set<string>() },
      prevTaskIdRef: { current: "task-old" as string | null },
      prevSessionIdRef: { current: "session-old" as string | null },
    };

    const previousApi = useDockviewStore.getState().api;
    useDockviewStore.setState({ api });
    try {
      runAutoSessionTabEffect("session-pending", appStore as never, refs as never);
    } finally {
      useDockviewStore.setState({ api: previousApi });
    }

    expect(refs.prevTaskIdRef.current).toBe("task-A");
    expect(refs.prevSessionIdRef.current).toBe("session-pending");
  });
});
