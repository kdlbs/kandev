import type { DockviewApi, SerializedDockview } from "dockview-react";
import type { LayoutNode, LayoutState } from "./layout-manager";
import { CENTER_GROUP } from "./layout-manager";

type ActiveViewEntry = { groupId: string; activeView: string };

type ActiveViewTarget = {
  entries: ActiveViewEntry[];
  activeGroup?: string;
};

type LayoutGroup = LayoutState["columns"][number]["groups"][number];

function collectSavedActiveViews(
  saved: SerializedDockview,
): Array<{ groupId: string; activeView: string }> {
  const out: Array<{ groupId: string; activeView: string }> = [];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const walk = (node: any): void => {
    if (!node) return;
    if (Array.isArray(node.data)) {
      for (const child of node.data) walk(child);
      return;
    }
    const data = node.data;
    if (data?.id && data.activeView) out.push({ groupId: data.id, activeView: data.activeView });
  };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  walk((saved as any).grid?.root);
  return out;
}

function collectLayoutGroups(layout: LayoutState): LayoutGroup[] {
  const groups: LayoutGroup[] = [];
  const visit = (node: LayoutNode): void => {
    if (node.type === "leaf") {
      groups.push(node.group);
      return;
    }
    for (const child of node.children) visit(child);
  };

  for (const column of layout.columns) {
    if (column.tree) {
      visit(column.tree);
      continue;
    }
    groups.push(...column.groups);
  }
  return groups;
}

function collectDefaultActiveViews(layout: LayoutState, liveLayout: LayoutState): ActiveViewTarget {
  const liveGroups = collectLayoutGroups(liveLayout);
  const liveGroupIds = new Set(liveGroups.flatMap((group) => (group.id ? [group.id] : [])));
  const entries: Array<ActiveViewEntry & { explicit: boolean }> = [];
  let explicitActiveGroup: string | undefined;
  let groupIndex = 0;

  const addGroup = (group: LayoutGroup): void => {
    const liveGroup = liveGroups[groupIndex++];
    const groupId =
      (group.id && liveGroupIds.has(group.id) ? group.id : undefined) ?? liveGroup?.id;
    const activeView = group.activePanel ?? group.panels[0]?.id;
    if (!groupId || !activeView) return;
    const explicit = !!group.activePanel;
    entries.push({ groupId, activeView, explicit });
    if (explicit && !explicitActiveGroup) explicitActiveGroup = groupId;
  };

  for (const group of collectLayoutGroups(layout)) addGroup(group);

  return {
    entries,
    activeGroup:
      explicitActiveGroup ?? entries.find((entry) => entry.groupId === CENTER_GROUP)?.groupId,
  };
}

function isSemanticAgentView(activeView: string): boolean {
  return activeView === "chat" || activeView.startsWith("session:");
}

function resolveTargetPanel(
  group: DockviewApi["groups"][number],
  activeView: string,
  activeSessionId: string | null,
): DockviewApi["panels"][number] | undefined {
  if (activeSessionId && isSemanticAgentView(activeView)) {
    const incomingSessionPanel = group.panels.find(
      (panel) => panel.id === `session:${activeSessionId}`,
    );
    if (incomingSessionPanel) return incomingSessionPanel;
  }
  return group.panels.find((panel) => panel.id === activeView);
}

/**
 * Restore each group's selected panel from the target layout. Agent panel IDs
 * are dynamic, so saved `chat` or stale `session:*` selections resolve to the
 * incoming session panel after session reconciliation. The fast path does not
 * call `fromJSON`, and the slow path replaces stale sessions after it does;
 * both paths therefore need this same post-reconciliation replay.
 *
 * The target `activeGroup` is applied last so the resulting global focus
 * matches what was persisted (or the effective default's explicit Agent
 * selection).
 */
function restoreTargetActiveViews(
  api: DockviewApi,
  target: ActiveViewTarget,
  activeSessionId: string | null,
): void {
  const ordered = target.activeGroup
    ? [
        ...target.entries.filter((entry) => entry.groupId !== target.activeGroup),
        ...target.entries.filter((entry) => entry.groupId === target.activeGroup),
      ]
    : target.entries;
  for (const { groupId, activeView } of ordered) {
    const group = api.groups.find((g) => g.id === groupId);
    if (!group) continue;
    const panel = resolveTargetPanel(group, activeView, activeSessionId);
    if (panel) {
      try {
        panel.api.setActive();
      } catch {
        /* panel may be in a transient state */
      }
    }
  }
}

export function restoreSavedActiveViews(
  api: DockviewApi,
  saved: SerializedDockview,
  activeSessionId: string | null,
): void {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const activeGroup = (saved as any).activeGroup as string | undefined;
  restoreTargetActiveViews(
    api,
    { entries: collectSavedActiveViews(saved), activeGroup },
    activeSessionId,
  );
}

export function restoreDefaultActiveViews(
  api: DockviewApi,
  defaultLayout: LayoutState,
  liveLayout: LayoutState,
  activeSessionId: string | null,
): void {
  restoreTargetActiveViews(
    api,
    collectDefaultActiveViews(defaultLayout, liveLayout),
    activeSessionId,
  );
}
