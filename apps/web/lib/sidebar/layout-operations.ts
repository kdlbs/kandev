import { generateUUID } from "@/lib/utils";
import {
  SIDEBAR_LAYOUT_LIMITS,
  SIDEBAR_LAYOUT_VERSION,
  targetKey,
  type SidebarLayout,
  type SidebarLayoutNode,
  type SidebarShortcut,
} from "./layout-types";

export type SidebarLayoutErrorCode =
  | "invalid_version"
  | "invalid_group_name"
  | "missing_node"
  | "not_a_group"
  | "duplicate_target"
  | "duplicate_id"
  | "protected_node"
  | "limit_reached"
  | "invalid_position";

export type SidebarLayoutValidation = {
  valid: boolean;
  errors: SidebarLayoutErrorCode[];
};

export class SidebarLayoutOperationError extends Error {
  readonly code: SidebarLayoutErrorCode;

  constructor(code: SidebarLayoutErrorCode) {
    super(code);
    this.name = "SidebarLayoutOperationError";
    this.code = code;
  }
}

const PROTECTED_NODE_IDS = new Set(["tasks", "inbox", "needs-you-inbox"]);
function copyLayout(layout: SidebarLayout): SidebarLayout {
  return {
    ...layout,
    nodes: layout.nodes.map((node) => ({
      ...node,
      ...(node.shortcuts ? { shortcuts: node.shortcuts.map((shortcut) => ({ ...shortcut })) } : {}),
    })),
  };
}

function codePoints(value: string): number {
  return Array.from(value).length;
}

function groupAt(layout: SidebarLayout, nodeId: string): SidebarLayoutNode {
  const node = layout.nodes.find((candidate) => candidate.id === nodeId);
  if (!node) throw new SidebarLayoutOperationError("missing_node");
  if (node.kind !== "shortcuts") throw new SidebarLayoutOperationError("not_a_group");
  return node;
}

function shortcutCount(layout: SidebarLayout): number {
  return layout.nodes.reduce((total, node) => total + (node.shortcuts?.length ?? 0), 0);
}

function validateShortcutGroup(node: SidebarLayoutNode): SidebarLayoutErrorCode[] {
  const errors: SidebarLayoutErrorCode[] = [];
  const name = node.name?.trim() ?? "";
  if (codePoints(name) === 0 || codePoints(name) > SIDEBAR_LAYOUT_LIMITS.sectionNameCodePoints) {
    errors.push("invalid_group_name");
  }
  if ((node.shortcuts?.length ?? 0) > SIDEBAR_LAYOUT_LIMITS.shortcutsPerGroup) {
    errors.push("limit_reached");
  }
  const shortcutIds = new Set<string>();
  const targets = new Set<string>();
  for (const shortcut of node.shortcuts ?? []) {
    if (shortcutIds.has(shortcut.id)) errors.push("duplicate_id");
    shortcutIds.add(shortcut.id);
    const key = targetKey(shortcut.target);
    if (targets.has(key)) errors.push("duplicate_target");
    targets.add(key);
  }
  return errors;
}

function validateNode(node: SidebarLayoutNode, nodeIds: Set<string>): SidebarLayoutErrorCode[] {
  const errors: SidebarLayoutErrorCode[] = [];
  if (nodeIds.has(node.id)) errors.push("duplicate_id");
  nodeIds.add(node.id);
  if (node.kind === "shortcuts") return [...errors, ...validateShortcutGroup(node)];
  if (node.id.startsWith("tasks") || PROTECTED_NODE_IDS.has(node.id)) errors.push("protected_node");
  return errors;
}

export function validateSidebarLayout(layout: SidebarLayout): SidebarLayoutValidation {
  const errors: SidebarLayoutErrorCode[] = [];
  if (layout.version !== SIDEBAR_LAYOUT_VERSION) errors.push("invalid_version");
  if (
    layout.nodes.filter((node) => node.kind === "shortcuts").length > SIDEBAR_LAYOUT_LIMITS.groups
  ) {
    errors.push("limit_reached");
  }
  const nodeIds = new Set<string>();
  for (const node of layout.nodes) errors.push(...validateNode(node, nodeIds));
  if (shortcutCount(layout) > SIDEBAR_LAYOUT_LIMITS.shortcuts) errors.push("limit_reached");
  return { valid: errors.length === 0, errors: [...new Set(errors)] };
}

function replaceNode(
  layout: SidebarLayout,
  nodeId: string,
  replace: (node: SidebarLayoutNode) => SidebarLayoutNode,
) {
  const next = copyLayout(layout);
  const index = next.nodes.findIndex((node) => node.id === nodeId);
  if (index < 0) throw new SidebarLayoutOperationError("missing_node");
  next.nodes[index] = replace(next.nodes[index]);
  return next;
}

export function createShortcutSection(
  layout: SidebarLayout,
  name: string,
  id = generateUUID(),
): SidebarLayout {
  const trimmed = name.trim();
  if (
    codePoints(trimmed) === 0 ||
    codePoints(trimmed) > SIDEBAR_LAYOUT_LIMITS.sectionNameCodePoints
  ) {
    throw new SidebarLayoutOperationError("invalid_group_name");
  }
  if (
    layout.nodes.filter((node) => node.kind === "shortcuts").length >= SIDEBAR_LAYOUT_LIMITS.groups
  ) {
    throw new SidebarLayoutOperationError("limit_reached");
  }
  if (layout.nodes.some((node) => node.id === id))
    throw new SidebarLayoutOperationError("duplicate_id");
  const next = copyLayout(layout);
  next.nodes.push({ id, kind: "shortcuts", visible: true, name: trimmed, shortcuts: [] });
  return next;
}

export function renameShortcutSection(
  layout: SidebarLayout,
  nodeId: string,
  name: string,
): SidebarLayout {
  const trimmed = name.trim();
  if (
    codePoints(trimmed) === 0 ||
    codePoints(trimmed) > SIDEBAR_LAYOUT_LIMITS.sectionNameCodePoints
  ) {
    throw new SidebarLayoutOperationError("invalid_group_name");
  }
  groupAt(layout, nodeId);
  return replaceNode(layout, nodeId, (node) => ({ ...node, name: trimmed }));
}

/**
 * Applies the value currently in a section name input without validating or
 * normalizing it. Validation belongs to the commit boundary so a user can
 * clear and retype a name, and spaces typed between words are not discarded.
 */
export function setShortcutSectionNameDraft(
  layout: SidebarLayout,
  nodeId: string,
  name: string,
): SidebarLayout {
  groupAt(layout, nodeId);
  return replaceNode(layout, nodeId, (node) => ({ ...node, name }));
}

export function removeShortcutSection(layout: SidebarLayout, nodeId: string): SidebarLayout {
  if (PROTECTED_NODE_IDS.has(nodeId)) throw new SidebarLayoutOperationError("protected_node");
  groupAt(layout, nodeId);
  return {
    ...copyLayout(layout),
    nodes: layout.nodes.filter((node) => node.id !== nodeId),
  };
}

export function toggleNodeVisibility(
  layout: SidebarLayout,
  nodeId: string,
  visible: boolean,
): SidebarLayout {
  if (PROTECTED_NODE_IDS.has(nodeId)) throw new SidebarLayoutOperationError("protected_node");
  return replaceNode(layout, nodeId, (node) => ({ ...node, visible }));
}

export function addShortcut(
  layout: SidebarLayout,
  nodeId: string,
  shortcut: SidebarShortcut,
): SidebarLayout {
  const group = groupAt(layout, nodeId);
  if (layout.nodes.some((node) => node.shortcuts?.some((item) => item.id === shortcut.id))) {
    throw new SidebarLayoutOperationError("duplicate_id");
  }
  if (shortcutCount(layout) >= SIDEBAR_LAYOUT_LIMITS.shortcuts) {
    throw new SidebarLayoutOperationError("limit_reached");
  }
  if ((group.shortcuts?.length ?? 0) >= SIDEBAR_LAYOUT_LIMITS.shortcutsPerGroup) {
    throw new SidebarLayoutOperationError("limit_reached");
  }
  if (group.shortcuts?.some((item) => targetKey(item.target) === targetKey(shortcut.target))) {
    throw new SidebarLayoutOperationError("duplicate_target");
  }
  return replaceNode(layout, nodeId, (node) => ({
    ...node,
    shortcuts: [...(node.shortcuts ?? []), { ...shortcut }],
  }));
}

export function removeShortcut(
  layout: SidebarLayout,
  nodeId: string,
  shortcutId: string,
): SidebarLayout {
  const group = groupAt(layout, nodeId);
  if (!group.shortcuts?.some((shortcut) => shortcut.id === shortcutId)) {
    throw new SidebarLayoutOperationError("missing_node");
  }
  return replaceNode(layout, nodeId, (node) => ({
    ...node,
    shortcuts: (node.shortcuts ?? []).filter((shortcut) => shortcut.id !== shortcutId),
  }));
}

function assertMovePosition(position: number): void {
  if (!Number.isInteger(position) || position < 0) {
    throw new SidebarLayoutOperationError("invalid_position");
  }
}

function assertTargetIsUnique(
  sourceNodeId: string,
  destinationNodeId: string,
  destinationItems: SidebarShortcut[],
  item: SidebarShortcut,
): void {
  const duplicate = destinationItems.some(
    (shortcut) => targetKey(shortcut.target) === targetKey(item.target),
  );
  if (sourceNodeId !== destinationNodeId && duplicate) {
    throw new SidebarLayoutOperationError("duplicate_target");
  }
}

function insertMovedShortcut({
  layout,
  sourceNodeId,
  destinationNodeId,
  sourceIndex,
  position,
  item,
}: {
  layout: SidebarLayout;
  sourceNodeId: string;
  destinationNodeId: string;
  sourceIndex: number;
  position: number;
  item: SidebarShortcut;
}): SidebarLayout {
  const next = copyLayout(layout);
  const source = groupAt(next, sourceNodeId);
  const destination = groupAt(next, destinationNodeId);
  const sourceItems = source.shortcuts ?? [];
  const destinationItems = destination.shortcuts ?? [];
  sourceItems.splice(sourceIndex, 1);
  const targetPosition =
    sourceNodeId === destinationNodeId && position > sourceIndex ? position - 1 : position;
  if (targetPosition > destinationItems.length) {
    throw new SidebarLayoutOperationError("invalid_position");
  }
  destination.shortcuts = destinationItems;
  destinationItems.splice(targetPosition, 0, item);
  return next;
}

export function moveShortcut(
  layout: SidebarLayout,
  shortcutId: string,
  sourceNodeId: string,
  destinationNodeId: string,
  position: number,
): SidebarLayout {
  const source = groupAt(layout, sourceNodeId);
  const destination = groupAt(layout, destinationNodeId);
  const sourceItems = source.shortcuts ?? [];
  const destinationItems = destination.shortcuts ?? [];
  const sourceIndex = sourceItems.findIndex((shortcut) => shortcut.id === shortcutId);
  const item = sourceItems[sourceIndex];
  if (!item) throw new SidebarLayoutOperationError("missing_node");
  assertMovePosition(position);
  if (
    sourceNodeId !== destinationNodeId &&
    destinationItems.length >= SIDEBAR_LAYOUT_LIMITS.shortcutsPerGroup
  ) {
    throw new SidebarLayoutOperationError("limit_reached");
  }
  assertTargetIsUnique(sourceNodeId, destinationNodeId, destinationItems, item);
  return insertMovedShortcut({
    layout,
    sourceNodeId,
    destinationNodeId,
    sourceIndex,
    position,
    item,
  });
}

export function moveSection(
  layout: SidebarLayout,
  nodeId: string,
  position: number,
): SidebarLayout {
  if (PROTECTED_NODE_IDS.has(nodeId)) throw new SidebarLayoutOperationError("protected_node");
  if (!Number.isInteger(position) || position < 0 || position >= layout.nodes.length) {
    throw new SidebarLayoutOperationError("invalid_position");
  }
  const index = layout.nodes.findIndex((node) => node.id === nodeId);
  if (index < 0) throw new SidebarLayoutOperationError("missing_node");
  const next = copyLayout(layout);
  const [node] = next.nodes.splice(index, 1);
  if (!node) throw new SidebarLayoutOperationError("missing_node");
  next.nodes.splice(position, 0, node);
  return next;
}
