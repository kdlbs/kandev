"use client";

import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import {
  addShortcut,
  createShortcutSection,
  moveSection,
  moveShortcut,
  removeShortcut,
  removeShortcutSection,
  toggleNodeVisibility,
} from "@/lib/sidebar/layout-operations";
import type { SidebarLayout, SidebarLayoutNode } from "@/lib/sidebar/layout-types";
import type { ProjectedSidebarNode } from "@/lib/sidebar/layout-projection";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import { generateUUID } from "@/lib/utils";
import type { DraftOperation } from "./sidebar-layout-editor-types";
import { SidebarLayoutNodeEditor } from "./sidebar-layout-editor-node-item";
import {
  handleSidebarLayoutDragEnd,
  moveShortcutToSection,
  nodeDragId,
} from "./sidebar-layout-editor-dnd";

const SECTION_NAME_LABEL_KEY = "settings:sectionName";

function isEditableGroup(node: SidebarLayoutNode): boolean {
  return node.kind === "shortcuts";
}

function SidebarLayoutFeedback({
  readOnly,
  validation,
  validationMessage,
  operationError,
  saveError,
  latestLoading,
  onLoadLatest,
  t,
}: Pick<
  NodesCardProps,
  | "readOnly"
  | "validation"
  | "validationMessage"
  | "operationError"
  | "saveError"
  | "latestLoading"
  | "onLoadLatest"
  | "t"
>) {
  return (
    <>
      {readOnly && (
        <div className="flex flex-wrap items-center gap-2" role="alert">
          <p className="text-sm text-destructive">{t("settings:sidebarUnsupportedVersion")}</p>
          <Button
            type="button"
            variant="outline"
            className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={onLoadLatest}
            disabled={latestLoading}
          >
            {latestLoading ? t("common:loading") : t("settings:sidebarLoadLatest")}
          </Button>
        </div>
      )}
      {!validation.valid && (
        <p className="text-sm text-destructive" role="alert">
          {validationMessage}
        </p>
      )}
      {operationError && (
        <p className="text-sm text-destructive" role="alert">
          {operationError}
        </p>
      )}
      {saveError && (
        <div className="flex flex-wrap items-center gap-2" role="alert">
          <p className="text-sm text-destructive">
            {t(saveError === "conflict" ? "settings:sidebarConflict" : "settings:sidebarSaveError")}
          </p>
          {saveError === "conflict" && (
            <Button
              type="button"
              variant="outline"
              className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={onLoadLatest}
              disabled={latestLoading}
            >
              {latestLoading ? t("common:loading") : t("settings:sidebarLoadLatest")}
            </Button>
          )}
        </div>
      )}
    </>
  );
}

type NodesCardProps = {
  draft: SidebarLayout;
  readOnly: boolean;
  projected: { nodes: ProjectedSidebarNode[] };
  catalog: ShortcutCatalogEntry[];
  catalogLoading: boolean;
  catalogError: string | null;
  canvasError?: string | null;
  newSectionName: string;
  setNewSectionName: (value: string) => void;
  pickerNodeId: string | null;
  setPickerNodeId: (value: string | null) => void;
  pickerQuery: string;
  setPickerQuery: (value: string) => void;
  validation: { valid: boolean; errors: string[] };
  validationMessage: string | null;
  operationError: string | null;
  saveError: "conflict" | "error" | null;
  latestLoading: boolean;
  onLoadLatest: () => void;
  onApply: (operation: DraftOperation) => boolean;
  onSetNameDraft: (nodeId: string, name: string) => void;
  onCommitName: (nodeId: string, name: string) => void;
  onSetFocusedNodeId: (value: string | null) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

function NewShortcutSectionForm({
  readOnly,
  name,
  setName,
  onApply,
  t,
}: {
  readOnly: boolean;
  name: string;
  setName: (value: string) => void;
  onApply: (operation: DraftOperation) => boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  return (
    <div className="flex flex-col gap-2 rounded-md border border-dashed p-3 sm:flex-row">
      <Input
        aria-label={t(SECTION_NAME_LABEL_KEY)}
        value={name}
        onChange={(event) => setName(event.target.value)}
        disabled={readOnly}
        placeholder={t(SECTION_NAME_LABEL_KEY)}
        className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      />
      <Button
        type="button"
        variant="outline"
        className="min-h-7 shrink-0 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={() => {
          if (onApply((current) => createShortcutSection(current, name))) setName("");
        }}
        disabled={readOnly || !name.trim()}
      >
        <IconPlus className="mr-2 h-4 w-4" />
        {t("settings:addShortcutSection")}
      </Button>
    </div>
  );
}

type NodeListProps = Pick<
  NodesCardProps,
  | "draft"
  | "projected"
  | "catalog"
  | "catalogLoading"
  | "catalogError"
  | "canvasError"
  | "readOnly"
  | "pickerNodeId"
  | "pickerQuery"
  | "setPickerNodeId"
  | "setPickerQuery"
  | "onApply"
  | "onSetNameDraft"
  | "onCommitName"
  | "onSetFocusedNodeId"
  | "t"
>;

function SidebarLayoutNodeList({
  draft,
  projected,
  catalog,
  catalogLoading,
  catalogError,
  canvasError,
  readOnly,
  pickerNodeId,
  pickerQuery,
  setPickerNodeId,
  setPickerQuery,
  onApply,
  onSetNameDraft,
  onCommitName,
  onSetFocusedNodeId,
  t,
}: NodeListProps) {
  const sections = draft.nodes.filter(isEditableGroup);
  return (
    <>
      {draft.nodes.map((node, index) => (
        <SidebarLayoutNodeEditor
          key={node.id}
          node={node}
          index={index}
          total={draft.nodes.length}
          projected={projected.nodes.find((item) => item.id === node.id)}
          sections={sections}
          catalog={catalog}
          loading={catalogLoading}
          catalogError={catalogError}
          canvasError={canvasError}
          readOnly={readOnly}
          pickerOpen={pickerNodeId === node.id}
          query={pickerQuery}
          onQueryChange={setPickerQuery}
          onToggle={() =>
            onApply((current) => toggleNodeVisibility(current, node.id, !node.visible))
          }
          onMove={(position) => onApply((current) => moveSection(current, node.id, position))}
          onRemove={() => onApply((current) => removeShortcutSection(current, node.id))}
          onSetNameDraft={onSetNameDraft}
          onCommitName={onCommitName}
          onOpenPicker={() => setPickerNodeId(node.id)}
          onClosePicker={() => setPickerNodeId(null)}
          onAdd={(entry) => {
            if (
              onApply((current) =>
                addShortcut(current, node.id, { id: generateUUID(), target: entry.target }),
              )
            ) {
              setPickerQuery("");
            }
          }}
          onRemoveShortcut={(shortcutId) =>
            onApply((current) => removeShortcut(current, node.id, shortcutId))
          }
          onMoveShortcut={(shortcutId, position) =>
            onApply((current) => moveShortcut(current, shortcutId, node.id, node.id, position))
          }
          onMoveShortcutToSection={(shortcutId, destinationNodeId) =>
            onApply((current) =>
              moveShortcutToSection(current, shortcutId, node.id, destinationNodeId),
            )
          }
          onFocus={() => onSetFocusedNodeId(node.id)}
          t={t}
        />
      ))}
    </>
  );
}

export function SidebarLayoutNodesCard({
  draft,
  readOnly,
  projected,
  catalog,
  catalogLoading,
  catalogError,
  canvasError,
  newSectionName,
  setNewSectionName,
  pickerNodeId,
  setPickerNodeId,
  pickerQuery,
  setPickerQuery,
  validation,
  validationMessage,
  operationError,
  saveError,
  latestLoading,
  onLoadLatest,
  onApply,
  onSetNameDraft,
  onCommitName,
  onSetFocusedNodeId,
  t,
}: NodesCardProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings:sidebarLayout")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <DndContext
          sensors={readOnly ? [] : sensors}
          collisionDetection={closestCenter}
          onDragEnd={(event) => {
            if (!readOnly) handleSidebarLayoutDragEnd(event, draft, onApply);
          }}
        >
          <SortableContext
            items={draft.nodes.map((node) => nodeDragId(node.id))}
            strategy={verticalListSortingStrategy}
          >
            <SidebarLayoutNodeList
              draft={draft}
              projected={projected}
              catalog={catalog}
              catalogLoading={catalogLoading}
              catalogError={catalogError}
              canvasError={canvasError}
              readOnly={readOnly}
              pickerNodeId={pickerNodeId}
              pickerQuery={pickerQuery}
              setPickerNodeId={setPickerNodeId}
              setPickerQuery={setPickerQuery}
              onApply={onApply}
              onSetNameDraft={onSetNameDraft}
              onCommitName={onCommitName}
              onSetFocusedNodeId={onSetFocusedNodeId}
              t={t}
            />
          </SortableContext>
        </DndContext>
        <NewShortcutSectionForm
          readOnly={readOnly}
          name={newSectionName}
          setName={setNewSectionName}
          onApply={onApply}
          t={t}
        />
        <SidebarLayoutFeedback
          readOnly={readOnly}
          validation={validation}
          validationMessage={validationMessage}
          operationError={operationError}
          saveError={saveError}
          latestLoading={latestLoading}
          onLoadLatest={onLoadLatest}
          t={t}
        />
      </CardContent>
    </Card>
  );
}
