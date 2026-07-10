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

function targetPanel(activeSessionId: string | null): LayoutPanel {
  return activeSessionId ? sessionPanel(activeSessionId) : knownPanel(CHAT_PANEL_ID);
}

function rewriteGroup(
  group: LayoutGroup,
  target: LayoutPanel,
  inserted: { value: boolean },
): LayoutGroup | null {
  let activeWasRewritten = false;
  const panels: LayoutPanel[] = [];

  for (const current of group.panels) {
    if (!isSessionChatPanel(current)) {
      panels.push(current);
      continue;
    }

    activeWasRewritten = activeWasRewritten || group.activePanel === current.id;
    if (!inserted.value) {
      panels.push(target);
      inserted.value = true;
    }
  }

  if (panels.length === 0) return null;
  const activeStillExists = group.activePanel && panels.some((p) => p.id === group.activePanel);
  const targetStillExists = panels.some((p) => p.id === target.id);
  let activePanel = group.activePanel;

  if (activeWasRewritten) {
    activePanel = targetStillExists ? target.id : panels[0]?.id;
  } else if (group.activePanel && !activeStillExists) {
    activePanel = panels[0]?.id;
  }

  return { ...group, panels, activePanel };
}

function collectGroupsFromTree(node: LayoutNode): LayoutGroup[] {
  if (node.type === "leaf") return [node.group];
  return node.children.flatMap(collectGroupsFromTree);
}

function rewriteTreeNode(
  node: LayoutNode,
  target: LayoutPanel,
  inserted: { value: boolean },
): LayoutNode | null {
  if (node.type === "leaf") {
    const group = rewriteGroup(node.group, target, inserted);
    return group ? { ...node, group } : null;
  }

  const children = node.children
    .map((child) => rewriteTreeNode(child, target, inserted))
    .filter((child): child is LayoutNode => child !== null);
  if (children.length === 0) return null;
  return { ...node, children };
}

function rewriteColumn(
  column: LayoutColumn,
  target: LayoutPanel,
  inserted: { value: boolean },
): LayoutColumn | null {
  if (column.tree) {
    const tree = rewriteTreeNode(column.tree, target, inserted);
    if (!tree) return null;
    return { ...column, tree, groups: collectGroupsFromTree(tree) };
  }

  if (!Array.isArray(column.groups)) {
    return column;
  }

  const groups = column.groups
    .map((group) => rewriteGroup(group, target, inserted))
    .filter((group): group is LayoutGroup => group !== null);
  if (groups.length === 0) return null;
  return { ...column, groups };
}

function rewriteReusableChatPanels(state: LayoutState, target: LayoutPanel): LayoutState {
  const inserted = { value: false };
  const columns = state.columns
    .map((column) => rewriteColumn(column, target, inserted))
    .filter((column): column is LayoutColumn => column !== null);
  return { columns };
}

export function normalizeReusableSessionPanels(state: LayoutState): LayoutState {
  return rewriteReusableChatPanels(state, knownPanel(CHAT_PANEL_ID));
}

export function materializeReusableChatPanel(
  state: LayoutState,
  activeSessionId: string | null,
): LayoutState {
  return rewriteReusableChatPanels(state, targetPanel(activeSessionId));
}
