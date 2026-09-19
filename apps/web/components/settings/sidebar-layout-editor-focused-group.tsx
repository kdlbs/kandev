"use client";

import { useTranslation } from "react-i18next";
import {
  IconArrowLeft,
  IconChevronDown,
  IconChevronUp,
  IconPlus,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { SettingsPageHeader } from "@/components/settings/settings-typography";
import { moveShortcut, removeShortcut } from "@/lib/sidebar/layout-operations";
import type { SidebarLayout } from "@/lib/sidebar/layout-types";
import type { ProjectedSidebarNode } from "@/lib/sidebar/layout-projection";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { DraftOperation } from "./sidebar-layout-editor-types";
import {
  SidebarShortcutPicker,
  ShortcutSectionMoveMenu,
} from "./sidebar-layout-editor-shortcut-picker";

const isEditableGroup = (node: SidebarLayout["nodes"][number]): boolean =>
  node.kind === "shortcuts";

function FocusedShortcutRows({
  node,
  sections,
  onApply,
  t,
}: {
  node: ProjectedSidebarNode;
  sections: SidebarLayout["nodes"];
  onApply: (operation: DraftOperation) => boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  return (
    <div className="space-y-2">
      {node.shortcuts.map((shortcut, index) => {
        const Icon = shortcut.icon;
        return (
          <div
            key={shortcut.id}
            className="flex min-h-11 items-center gap-2 rounded-md border px-3"
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span className="min-w-0 flex-1 truncate">{shortcut.label}</span>
            <ShortcutSectionMoveMenu
              shortcut={shortcut}
              currentNodeId={node.id}
              sections={sections}
              onMove={(destinationNodeId) =>
                onApply((current) => {
                  const destination = current.nodes.find((item) => item.id === destinationNodeId);
                  return moveShortcut(
                    current,
                    shortcut.id,
                    node.id,
                    destinationNodeId,
                    destination?.shortcuts?.length ?? 0,
                  );
                })
              }
              t={t}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-11"
              onClick={() =>
                onApply((current) =>
                  moveShortcut(current, shortcut.id, node.id, node.id, index - 1),
                )
              }
              disabled={index === 0}
              aria-label={t("settings:moveUp")}
            >
              <IconChevronUp className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-11"
              onClick={() =>
                onApply((current) =>
                  moveShortcut(current, shortcut.id, node.id, node.id, index + 1),
                )
              }
              disabled={index === node.shortcuts.length - 1}
              aria-label={t("settings:moveDown")}
            >
              <IconChevronDown className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-11"
              onClick={() => onApply((current) => removeShortcut(current, node.id, shortcut.id))}
              aria-label={t("settings:remove")}
            >
              <IconTrash className="h-4 w-4" />
            </Button>
          </div>
        );
      })}
    </div>
  );
}

export function FocusedSidebarGroup({
  node,
  draft,
  catalog,
  loading,
  error,
  query,
  onQueryChange,
  onBack,
  onOpenPicker,
  pickerOpen,
  onClosePicker,
  onAdd,
  onApply,
  onSetNameDraft,
  onCommitName,
}: {
  node: ProjectedSidebarNode;
  draft: SidebarLayout;
  catalog: ShortcutCatalogEntry[];
  loading: boolean;
  error: string | null;
  query: string;
  onQueryChange: (value: string) => void;
  onBack: () => void;
  onOpenPicker: () => void;
  pickerOpen: boolean;
  onClosePicker: () => void;
  onAdd: (entry: ShortcutCatalogEntry) => void;
  onApply: (operation: DraftOperation) => boolean;
  onSetNameDraft: (nodeId: string, name: string) => void;
  onCommitName: (nodeId: string, name: string) => void;
}) {
  const { t } = useTranslation();
  const sections = draft.nodes.filter(isEditableGroup);
  return (
    <div
      className="flex min-h-[100dvh] min-w-0 flex-col gap-4 pb-[calc(6rem+env(safe-area-inset-bottom))]"
      data-testid="sidebar-layout-focused-group"
    >
      <Button type="button" variant="ghost" className="min-h-11 w-fit" onClick={onBack}>
        <IconArrowLeft className="mr-2 h-4 w-4" />
        {t("common:back")}
      </Button>
      <SettingsPageHeader title={node.name ?? ""} />
      <Input
        aria-label={t("settings:sectionName")}
        value={node.name ?? ""}
        onChange={(event) => onSetNameDraft(node.id, event.target.value)}
        onBlur={() => onCommitName(node.id, node.name ?? "")}
        className="min-h-11"
      />
      <FocusedShortcutRows node={node} sections={sections} onApply={onApply} t={t} />
      <Button type="button" variant="outline" className="min-h-11 w-full" onClick={onOpenPicker}>
        <IconPlus className="mr-2 h-4 w-4" />
        {t("settings:addShortcut")}
      </Button>
      {pickerOpen && (
        <SidebarShortcutPicker
          catalog={catalog}
          loading={loading}
          error={error}
          query={query}
          onQueryChange={onQueryChange}
          onAdd={onAdd}
          onClose={onClosePicker}
          existing={node.shortcuts.map(
            (shortcut) => `${shortcut.target.kind}:${shortcut.target.id}`,
          )}
          t={t}
        />
      )}
    </div>
  );
}
