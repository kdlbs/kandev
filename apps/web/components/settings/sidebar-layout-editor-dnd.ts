import type { DragEndEvent } from "@dnd-kit/core";
import { moveSection, moveShortcut } from "@/lib/sidebar/layout-operations";
import type { SidebarLayout } from "@/lib/sidebar/layout-types";
import type { DraftOperation } from "./sidebar-layout-editor-types";

export type SidebarLayoutDragData = {
  type: "node" | "shortcut" | "group";
  nodeId: string;
  shortcutId?: string;
};

export function nodeDragId(nodeId: string): string {
  return `sidebar-node:${encodeURIComponent(nodeId)}`;
}

export function groupDropId(nodeId: string): string {
  return `sidebar-group:${encodeURIComponent(nodeId)}`;
}

export function shortcutDragId(nodeId: string, shortcutId: string): string {
  return `sidebar-shortcut:${encodeURIComponent(nodeId)}|${encodeURIComponent(shortcutId)}`;
}

export function moveShortcutToSection(
  layout: SidebarLayout,
  shortcutId: string,
  sourceNodeId: string,
  destinationNodeId: string,
): SidebarLayout {
  const destination = layout.nodes.find((node) => node.id === destinationNodeId);
  return moveShortcut(
    layout,
    shortcutId,
    sourceNodeId,
    destinationNodeId,
    destination?.shortcuts?.length ?? 0,
  );
}

function applyNodeDrag(
  draft: SidebarLayout,
  active: SidebarLayoutDragData,
  over: SidebarLayoutDragData,
  onApply: (operation: DraftOperation) => boolean,
): void {
  if (over.type !== "node") return;
  const from = draft.nodes.findIndex((node) => node.id === active.nodeId);
  const to = draft.nodes.findIndex((node) => node.id === over.nodeId);
  if (from >= 0 && to >= 0 && from !== to) {
    onApply((current) => moveSection(current, active.nodeId, to));
  }
}

function applyShortcutDrag(
  draft: SidebarLayout,
  active: SidebarLayoutDragData,
  over: SidebarLayoutDragData,
  onApply: (operation: DraftOperation) => boolean,
): void {
  if (active.type !== "shortcut" || !active.shortcutId) return;
  if (over.type === "shortcut") {
    const destination = draft.nodes.find((node) => node.id === over.nodeId);
    const position =
      destination?.shortcuts?.findIndex((shortcut) => shortcut.id === over.shortcutId) ?? -1;
    if (position >= 0) {
      onApply((current) =>
        moveShortcut(current, active.shortcutId!, active.nodeId, over.nodeId, position),
      );
    }
    return;
  }
  if (over.type === "group") {
    onApply((current) =>
      moveShortcutToSection(current, active.shortcutId!, active.nodeId, over.nodeId),
    );
  }
}

export function handleSidebarLayoutDragEnd(
  event: DragEndEvent,
  draft: SidebarLayout,
  onApply: (operation: DraftOperation) => boolean,
): void {
  const active = event.active.data.current as SidebarLayoutDragData | undefined;
  const over = event.over?.data.current as SidebarLayoutDragData | undefined;
  if (!active || !over) return;
  if (active.type === "node") {
    applyNodeDrag(draft, active, over, onApply);
    return;
  }
  applyShortcutDrag(draft, active, over, onApply);
}
