"use client";

import type { HTMLAttributes } from "react";
import { useDroppable } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  IconChevronDown,
  IconChevronUp,
  IconGripVertical,
  IconPlus,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import type { SidebarLayoutNode } from "@/lib/sidebar/layout-types";
import type { ProjectedSidebarNode, ProjectedShortcut } from "@/lib/sidebar/layout-projection";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import {
  SidebarShortcutPicker,
  ShortcutSectionMoveMenu,
} from "./sidebar-layout-editor-shortcut-picker";
import {
  groupDropId,
  nodeDragId,
  shortcutDragId,
  type SidebarLayoutDragData,
} from "./sidebar-layout-editor-dnd";

const SECTION_NAME_LABEL_KEY = "settings:sectionName";
const BUILTIN_LABEL_KEYS: Record<string, string> = {
  home: "sidebar:home",
  new_task: "sidebar:newTask",
  automations: "common:automations",
  canvases: "canvases:canvases",
  integrations: "common:integrations",
};

function nodeTitle(node: SidebarLayoutNode, t: (key: string) => string): string {
  if (node.name) return node.name;
  return node.destinationId && BUILTIN_LABEL_KEYS[node.destinationId]
    ? t(BUILTIN_LABEL_KEYS[node.destinationId])
    : node.id;
}

function handleProps(
  attributes: object,
  listeners: object | undefined,
): HTMLAttributes<HTMLSpanElement> {
  return { ...attributes, ...(listeners ?? {}) } as HTMLAttributes<HTMLSpanElement>;
}

type NodeEditorProps = {
  node: SidebarLayoutNode;
  projected?: ProjectedSidebarNode;
  sections: SidebarLayoutNode[];
  index: number;
  total: number;
  catalog: ShortcutCatalogEntry[];
  loading: boolean;
  catalogError: string | null;
  pickerOpen: boolean;
  query: string;
  onQueryChange: (value: string) => void;
  onToggle: () => void;
  onMove: (position: number) => void;
  onRemove: () => void;
  onSetNameDraft: (nodeId: string, name: string) => void;
  onCommitName: (nodeId: string, name: string) => void;
  onOpenPicker: () => void;
  onClosePicker: () => void;
  onAdd: (entry: ShortcutCatalogEntry) => void;
  onRemoveShortcut: (shortcutId: string) => void;
  onMoveShortcut: (shortcutId: string, position: number) => void;
  onMoveShortcutToSection: (shortcutId: string, destinationNodeId: string) => void;
  onFocus: () => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

export function SidebarLayoutNodeEditor(props: NodeEditorProps) {
  const { node } = props;
  const sortable = useSortable({
    id: nodeDragId(node.id),
    data: { type: "node", nodeId: node.id } satisfies SidebarLayoutDragData,
  });
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };
  return (
    <div
      ref={sortable.setNodeRef}
      style={style}
      className="min-w-0 rounded-md border p-3"
      data-testid={`sidebar-layout-node-${node.id}`}
    >
      <SidebarLayoutNodeHeader
        {...props}
        dragHandleProps={handleProps(sortable.attributes, sortable.listeners)}
      />
      <SidebarLayoutShortcutList {...props} />
    </div>
  );
}

function SidebarLayoutNodeHeader({
  node,
  index,
  total,
  onToggle,
  onMove,
  onRemove,
  onSetNameDraft,
  onCommitName,
  dragHandleProps,
  t,
}: NodeEditorProps & { dragHandleProps: HTMLAttributes<HTMLSpanElement> }) {
  const isGroup = node.kind === "shortcuts";
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span
        {...dragHandleProps}
        className="flex h-7 w-7 shrink-0 cursor-grab items-center justify-center text-muted-foreground active:cursor-grabbing max-md:h-11 max-md:w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
        data-testid={`sidebar-layout-drag-handle-${node.id}`}
        aria-label={t("settings:dragToReorder")}
      >
        <IconGripVertical className="h-4 w-4" aria-hidden="true" />
      </span>
      {isGroup ? (
        <Input
          aria-label={t(SECTION_NAME_LABEL_KEY)}
          value={node.name ?? ""}
          onChange={(event) => onSetNameDraft(node.id, event.target.value)}
          onBlur={() => onCommitName(node.id, node.name ?? "")}
          className="min-h-7 min-w-0 flex-1 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        />
      ) : (
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{nodeTitle(node, t)}</span>
      )}
      <Switch
        checked={node.visible}
        onCheckedChange={onToggle}
        aria-label={t("settings:sidebarToggleVisibility")}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
        onClick={() => onMove(Math.max(0, index - 1))}
        disabled={index === 0}
        aria-label={t("settings:moveUp")}
      >
        <IconChevronUp className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
        onClick={() => onMove(Math.min(total - 1, index + 1))}
        disabled={index === total - 1}
        aria-label={t("settings:moveDown")}
      >
        <IconChevronDown className="h-4 w-4" />
      </Button>
      {isGroup && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
          onClick={onRemove}
          aria-label={t("settings:remove")}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}

function SidebarLayoutShortcutList({
  node,
  projected,
  sections,
  catalog,
  loading,
  catalogError,
  pickerOpen,
  query,
  onQueryChange,
  onOpenPicker,
  onClosePicker,
  onAdd,
  onRemoveShortcut,
  onMoveShortcut,
  onMoveShortcutToSection,
  onFocus,
  t,
}: NodeEditorProps) {
  const { setNodeRef } = useDroppable({
    id: groupDropId(node.id),
    data: { type: "group", nodeId: node.id } satisfies SidebarLayoutDragData,
  });
  if (node.kind !== "shortcuts") return null;
  const shortcuts = projected?.shortcuts ?? [];
  return (
    <div ref={setNodeRef} className="mt-3 space-y-2 pl-6">
      <SortableContext
        items={shortcuts.map((shortcut) => shortcutDragId(node.id, shortcut.id))}
        strategy={verticalListSortingStrategy}
      >
        {shortcuts.map((shortcut, shortcutIndex) => (
          <SortableSidebarShortcut
            key={shortcut.id}
            nodeId={node.id}
            shortcut={shortcut}
            shortcutIndex={shortcutIndex}
            shortcutCount={shortcuts.length}
            sections={sections}
            t={t}
            onMoveShortcut={onMoveShortcut}
            onRemoveShortcut={onRemoveShortcut}
            onMoveShortcutToSection={onMoveShortcutToSection}
          />
        ))}
      </SortableContext>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={onOpenPicker}
        >
          <IconPlus className="mr-2 h-4 w-4" />
          {t("settings:addShortcut")}
        </Button>
        <Button type="button" variant="ghost" className="min-h-11 md:hidden" onClick={onFocus}>
          {t("settings:editSection")}
        </Button>
      </div>
      {pickerOpen && (
        <SidebarShortcutPicker
          catalog={catalog}
          loading={loading}
          error={catalogError}
          query={query}
          onQueryChange={onQueryChange}
          onAdd={onAdd}
          onClose={onClosePicker}
          existing={shortcuts.map((shortcut) => `${shortcut.target.kind}:${shortcut.target.id}`)}
          t={t}
        />
      )}
    </div>
  );
}

function SortableSidebarShortcut({
  nodeId,
  shortcut,
  shortcutIndex,
  shortcutCount,
  sections,
  t,
  onMoveShortcut,
  onRemoveShortcut,
  onMoveShortcutToSection,
}: {
  nodeId: string;
  shortcut: ProjectedShortcut;
  shortcutIndex: number;
  shortcutCount: number;
  sections: SidebarLayoutNode[];
  t: (key: string, options?: Record<string, unknown>) => string;
  onMoveShortcut: (shortcutId: string, position: number) => void;
  onRemoveShortcut: (shortcutId: string) => void;
  onMoveShortcutToSection: (shortcutId: string, destinationNodeId: string) => void;
}) {
  const sortable = useSortable({
    id: shortcutDragId(nodeId, shortcut.id),
    data: { type: "shortcut", nodeId, shortcutId: shortcut.id } satisfies SidebarLayoutDragData,
  });
  const Icon = shortcut.icon;
  return (
    <div
      ref={sortable.setNodeRef}
      style={{
        transform: CSS.Transform.toString(sortable.transform),
        transition: sortable.transition,
      }}
      className="flex min-w-0 items-center gap-2 rounded-md bg-muted/30 p-2"
    >
      <span
        {...handleProps(sortable.attributes, sortable.listeners)}
        className="flex h-7 w-5 shrink-0 cursor-grab items-center justify-center text-muted-foreground active:cursor-grabbing max-md:h-11 max-md:w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
        aria-label={t("settings:dragToReorder")}
      >
        <IconGripVertical className="h-3.5 w-3.5" aria-hidden="true" />
      </span>
      <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate text-sm">{shortcut.label}</span>
      {!shortcut.available && (
        <span className="text-xs text-muted-foreground">{t("common:unavailable")}</span>
      )}
      <ShortcutSectionMoveMenu
        shortcut={shortcut}
        currentNodeId={nodeId}
        sections={sections}
        onMove={(destinationNodeId) => onMoveShortcutToSection(shortcut.id, destinationNodeId)}
        t={t}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
        onClick={() => onMoveShortcut(shortcut.id, Math.max(0, shortcutIndex - 1))}
        disabled={shortcutIndex === 0}
        aria-label={t("settings:moveUp")}
      >
        <IconChevronUp className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
        onClick={() => onMoveShortcut(shortcut.id, Math.min(shortcutCount - 1, shortcutIndex + 1))}
        disabled={shortcutIndex === shortcutCount - 1}
        aria-label={t("settings:moveDown")}
      >
        <IconChevronDown className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 max-md:size-11 [@media(pointer:coarse)]:size-11"
        onClick={() => onRemoveShortcut(shortcut.id)}
        aria-label={t("settings:remove")}
      >
        <IconX className="h-4 w-4" />
      </Button>
    </div>
  );
}
