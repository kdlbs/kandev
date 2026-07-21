import { panel as knownPanel } from "./constants";
import type { LayoutColumn, LayoutGroup, LayoutNode, LayoutPanel, LayoutState } from "./types";

const SESSION_PANEL_PREFIX = "session:";
const CHAT_COMPONENT = "chat";
const CHAT_PANEL_ID = "chat";

export function isSessionChatPanel(panel: LayoutPanel): boolean {
  return (
    panel.component === CHAT_COMPONENT &&
    (panel.id === CHAT_PANEL_ID || panel.id.startsWith(SESSION_PANEL_PREFIX))
  );
}

function sessionPanel(sessionId: string): LayoutPanel {
  return {
    id: `${SESSION_PANEL_PREFIX}${sessionId}`,
    component: CHAT_COMPONENT,
    title: "Agent",
    tabComponent: "sessionTab",
    params: { sessionId },
  };
}

type PanelTargets = { panels: LayoutPanel[]; activeId: string };

function targetPanels(activeSessionId: string | null, currentSessionIds: string[]): PanelTargets {
  if (!activeSessionId) {
    return { panels: [knownPanel(CHAT_PANEL_ID)], activeId: CHAT_PANEL_ID };
  }

  // currentSessionIds arrives already in step-flow order. Keep that order and
  // simply ensure the active session is present (a freshly-created active
  // session may not be in the list yet) by appending it — never force-prepend,
  // so the active tab stays in its step-flow slot instead of jumping to the
  // left edge.
  const withActive = currentSessionIds.includes(activeSessionId)
    ? currentSessionIds
    : [...currentSessionIds, activeSessionId];
  const orderedSessionIds = withActive.filter(
    (sessionId, index, sessionIds) => sessionId && sessionIds.indexOf(sessionId) === index,
  );
  return {
    panels: orderedSessionIds.map(sessionPanel),
    activeId: `${SESSION_PANEL_PREFIX}${activeSessionId}`,
  };
}

function rewrittenActivePanel(
  group: LayoutGroup,
  panels: LayoutPanel[],
  targets: PanelTargets,
  activeWasRewritten: boolean,
): string | undefined {
  if (activeWasRewritten) {
    const activeTargetId = panels.some((panel) => panel.id === targets.activeId)
      ? targets.activeId
      : panels[0]?.id;
    return activeTargetId;
  }

  if (group.activePanel && !panels.some((panel) => panel.id === group.activePanel)) {
    return panels[0]?.id;
  }

  return group.activePanel;
}

function rewriteGroup(
  group: LayoutGroup,
  targets: PanelTargets,
  inserted: { value: boolean },
): LayoutGroup | null {
  let activeWasRewritten = false;
  const panels: LayoutPanel[] = [];
  const hadPanels = group.panels.length > 0;

  for (const current of group.panels) {
    if (!isSessionChatPanel(current)) {
      panels.push(current);
      continue;
    }

    activeWasRewritten = activeWasRewritten || group.activePanel === current.id;
    // Keep only the first reusable chat slot. Later chat/session slots are
    // duplicate stale session tabs and should disappear with their group/column.
    if (!inserted.value) {
      panels.push(...targets.panels);
      inserted.value = true;
    }
  }

  if (panels.length === 0) return hadPanels ? null : { ...group, panels };
  return {
    ...group,
    panels,
    activePanel: rewrittenActivePanel(group, panels, targets, activeWasRewritten),
  };
}

function collectGroupsFromTree(node: LayoutNode): LayoutGroup[] {
  if (node.type === "leaf") return [node.group];
  return node.children.flatMap(collectGroupsFromTree);
}

function rewriteTreeNode(
  node: LayoutNode,
  targets: PanelTargets,
  inserted: { value: boolean },
): LayoutNode | null {
  if (node.type === "leaf") {
    const group = rewriteGroup(node.group, targets, inserted);
    return group ? { ...node, group } : null;
  }

  const children = node.children
    .map((child) => rewriteTreeNode(child, targets, inserted))
    .filter((child): child is LayoutNode => child !== null);
  if (children.length === 0) return null;
  return { ...node, children };
}

function rewriteColumn(
  column: LayoutColumn,
  targets: PanelTargets,
  inserted: { value: boolean },
): LayoutColumn | null {
  if (column.tree) {
    const tree = rewriteTreeNode(column.tree, targets, inserted);
    if (!tree) return null;
    return { ...column, tree, groups: collectGroupsFromTree(tree) };
  }

  if (!Array.isArray(column.groups)) {
    return column;
  }

  const groups = column.groups
    .map((group) => rewriteGroup(group, targets, inserted))
    .filter((group): group is LayoutGroup => group !== null);
  if (groups.length === 0) return null;
  return { ...column, groups };
}

function rewriteReusableChatPanels(state: LayoutState, targets: PanelTargets): LayoutState {
  const inserted = { value: false };
  const columns = state.columns
    .map((column) => rewriteColumn(column, targets, inserted))
    .filter((column): column is LayoutColumn => column !== null);
  return { columns };
}

export function normalizeReusableSessionPanels(state: LayoutState): LayoutState {
  return rewriteReusableChatPanels(state, {
    panels: [knownPanel(CHAT_PANEL_ID)],
    activeId: CHAT_PANEL_ID,
  });
}

export function materializeReusableChatPanel(
  state: LayoutState,
  activeSessionId: string | null,
  currentSessionIds: string[] = [],
): LayoutState {
  return rewriteReusableChatPanels(state, targetPanels(activeSessionId, currentSessionIds));
}
