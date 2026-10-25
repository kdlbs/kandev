"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useOfficeModeState } from "@/hooks/use-in-office";
import { useSidebarShortcutCatalog } from "@/hooks/domains/sidebar/use-sidebar-shortcut-catalog";
import {
  addShortcut,
  renameShortcutSection,
  validateSidebarLayout,
} from "@/lib/sidebar/layout-operations";
import { defaultSidebarLayout, type SidebarLayout } from "@/lib/sidebar/layout-types";
import {
  projectSidebarLayout,
  type SidebarLayoutProjection,
} from "@/lib/sidebar/layout-projection";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import { generateUUID } from "@/lib/utils";
import {
  BUILTIN_LAYOUT_NODES,
  builtinLayoutNodeLabelKey,
  type BuiltinLayoutContext,
} from "@/lib/sidebar/builtin-layout-nodes";
import { FocusedSidebarGroup } from "./sidebar-layout-editor-focused-group";
import { useSidebarLayoutSave } from "./sidebar-layout-editor-save";
import {
  useSidebarDraft,
  useSidebarLayoutActions,
  validationCopy,
} from "./sidebar-layout-editor-state";
import { SidebarLayoutContent } from "./sidebar-layout-editor-view";
import type { DraftOperation } from "./sidebar-layout-editor-types";

type EditorSurfaceProps = {
  draft: SidebarLayout;
  unsupportedVersion: boolean;
  projected: SidebarLayoutProjection;
  catalog: ShortcutCatalogEntry[];
  catalogLoading: boolean;
  catalogError: string | null;
  canvasError?: string | null;
  validation: ReturnType<typeof validateSidebarLayout>;
  saveError: "conflict" | "error" | null;
  latestLoading: boolean;
  operationError: string | null;
  isMobile: boolean;
  newSectionName: string;
  setNewSectionName: (value: string) => void;
  pickerNodeId: string | null;
  focusedNodeId: string | null;
  setPickerNodeId: (value: string | null) => void;
  pickerQuery: string;
  setPickerQuery: (value: string) => void;
  onApply: (operation: DraftOperation) => boolean;
  onSetNameDraft: (nodeId: string, name: string) => void;
  onCommitName: (nodeId: string, name: string) => void;
  onLoadLatest: () => void;
  onReset: () => void;
  onSetFocusedNodeId: (value: string | null) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
  showHeader: boolean;
};

function SidebarLayoutEditorSurface({
  draft,
  unsupportedVersion,
  projected,
  catalog,
  catalogLoading,
  catalogError,
  canvasError,
  validation,
  saveError,
  latestLoading,
  operationError,
  isMobile,
  newSectionName,
  setNewSectionName,
  pickerNodeId,
  focusedNodeId,
  setPickerNodeId,
  pickerQuery,
  setPickerQuery,
  onApply,
  onSetNameDraft,
  onCommitName,
  onLoadLatest,
  onReset,
  onSetFocusedNodeId,
  t,
  showHeader,
}: EditorSurfaceProps) {
  const visibleNodes = projected.nodes.filter((node) => node.visible && node.available !== false);
  const activeFocusedNode = projected.nodes.find((node) => node.id === focusedNodeId);
  if (isMobile && activeFocusedNode?.kind === "shortcuts") {
    return (
      <FocusedSidebarGroup
        node={activeFocusedNode}
        draft={draft}
        readOnly={unsupportedVersion}
        catalog={catalog}
        loading={catalogLoading}
        error={catalogError}
        canvasError={canvasError}
        query={pickerQuery}
        onQueryChange={setPickerQuery}
        onBack={() => onSetFocusedNodeId(null)}
        onLoadLatest={onLoadLatest}
        latestLoading={latestLoading}
        onOpenPicker={() => setPickerNodeId(activeFocusedNode.id)}
        pickerOpen={pickerNodeId === activeFocusedNode.id}
        onClosePicker={() => setPickerNodeId(null)}
        onAdd={(entry) => {
          if (
            onApply((current) =>
              addShortcut(current, activeFocusedNode.id, {
                id: generateUUID(),
                target: entry.target,
              }),
            )
          ) {
            setPickerNodeId(null);
          }
        }}
        onApply={onApply}
        onSetNameDraft={onSetNameDraft}
        onCommitName={onCommitName}
      />
    );
  }
  return (
    <SidebarLayoutContent
      draft={draft}
      readOnly={unsupportedVersion}
      projected={projected}
      visibleNodes={visibleNodes}
      catalog={catalog}
      catalogLoading={catalogLoading}
      catalogError={catalogError}
      canvasError={canvasError}
      newSectionName={newSectionName}
      setNewSectionName={setNewSectionName}
      pickerNodeId={pickerNodeId}
      setPickerNodeId={setPickerNodeId}
      pickerQuery={pickerQuery}
      setPickerQuery={setPickerQuery}
      validation={validation}
      validationMessage={validation.valid ? null : validationCopy(validation.errors[0] ?? "", t)}
      operationError={operationError ? validationCopy(operationError, t) : null}
      saveError={saveError}
      latestLoading={latestLoading}
      onLoadLatest={onLoadLatest}
      onApply={onApply}
      onSetNameDraft={onSetNameDraft}
      onCommitName={onCommitName}
      onReset={onReset}
      onSetFocusedNodeId={onSetFocusedNodeId}
      t={t}
      isMobile={isMobile}
      showHeader={showHeader}
    />
  );
}

function useSidebarBuiltinContext(workspaceId?: string): BuiltinLayoutContext {
  const coordinatorEnabled = useFeature("coordinator");
  const needsYouInboxEnabled = useFeature("needsYouInbox");
  const canvasesEnabled = useFeature("canvases");
  const inOffice = useOfficeModeState() === "office";
  return useMemo(
    () => ({
      hasWorkspace: Boolean(workspaceId),
      inOffice,
      features: {
        coordinator: coordinatorEnabled,
        canvases: canvasesEnabled,
        needsYouInbox: needsYouInboxEnabled,
      },
    }),
    [canvasesEnabled, coordinatorEnabled, inOffice, needsYouInboxEnabled, workspaceId],
  );
}

function useSidebarLayoutProjection(
  draft: SidebarLayout,
  catalog: ShortcutCatalogEntry[],
  builtinContext: BuiltinLayoutContext,
  t: ReturnType<typeof useTranslation>["t"],
): SidebarLayoutProjection {
  return useMemo(
    () =>
      projectSidebarLayout(draft, catalog, {
        unavailableLabel: t("common:unavailable"),
        builtinLabels: Object.fromEntries(
          BUILTIN_LAYOUT_NODES.map((node) => [
            node.destinationId,
            t(builtinLayoutNodeLabelKey(node, builtinContext)),
          ]),
        ),
        builtinContext,
      }),
    [builtinContext, catalog, draft, t],
  );
}

export function SidebarLayoutEditor({ embedded = false }: { embedded?: boolean } = {}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const catalogState = useSidebarShortcutCatalog();
  const draftState = useSidebarDraft(catalogState.catalog);
  const builtinContext = useSidebarBuiltinContext(draftState.workspaceId ?? undefined);
  const actions = useSidebarLayoutActions(draftState);
  const validation = validateSidebarLayout(draftState.draft);
  const save = useSidebarLayoutSave({
    workspaceId: draftState.workspaceId,
    draft: draftState.draft,
    dirty: draftState.dirty,
    validationValid: validation.valid,
    invalidReason: validation.valid ? undefined : validationCopy(validation.errors[0] ?? "", t),
    catalog: catalogState.catalog,
    setDraft: draftState.setDraft,
    acknowledge: draftState.acknowledge,
    setUserSettings: draftState.setUserSettings,
    store: draftState.store,
    draftRef: actions.draftRef,
    savedRef: useRefValue(draftState.saved),
    workspaceRef: actions.workspaceRef,
    generationsRef: actions.generationsRef,
    requestGenerationsRef: actions.requestGenerationsRef,
    onOperationError: actions.setOperationError,
  });
  const [newSectionName, setNewSectionName] = useState("");
  const [pickerNodeId, setPickerNodeId] = useState<string | null>(null);
  const [focusedNodeId, setFocusedNodeId] = useState<string | null>(null);
  const [pickerQuery, setPickerQuery] = useState("");
  useEffect(() => {
    setNewSectionName("");
    setPickerNodeId(null);
    setFocusedNodeId(null);
    setPickerQuery("");
  }, [draftState.workspaceId]);
  const applyDraftOperation = useCallback(
    (operation: DraftOperation) => {
      const applied = actions.applyDraftOperation(operation);
      if (applied) save.setSaveError(null);
      return applied;
    },
    [actions.applyDraftOperation, save.setSaveError],
  );
  const onCommitName = useCallback(
    (nodeId: string, name: string) => {
      applyDraftOperation((current) => renameShortcutSection(current, nodeId, name));
    },
    [applyDraftOperation],
  );
  const onReset = useCallback(
    () =>
      applyDraftOperation((current) => ({ ...defaultSidebarLayout(), revision: current.revision })),
    [applyDraftOperation],
  );
  const projected = useSidebarLayoutProjection(
    draftState.draft,
    catalogState.catalog,
    builtinContext,
    t,
  );

  if (!draftState.workspaceId) {
    return <p className="text-sm text-muted-foreground">{t("settings:sidebarNoWorkspace")}</p>;
  }
  return (
    <SidebarLayoutEditorSurface
      draft={draftState.draft}
      unsupportedVersion={Boolean(draftState.draft.unsupportedVersion)}
      projected={projected}
      catalog={catalogState.catalog}
      catalogLoading={catalogState.loading}
      catalogError={catalogState.error}
      canvasError={catalogState.canvasError}
      validation={validation}
      saveError={save.saveError}
      latestLoading={save.latestLoading}
      operationError={actions.operationError}
      isMobile={isMobile}
      newSectionName={newSectionName}
      setNewSectionName={setNewSectionName}
      pickerNodeId={pickerNodeId}
      focusedNodeId={focusedNodeId}
      setPickerNodeId={setPickerNodeId}
      pickerQuery={pickerQuery}
      setPickerQuery={setPickerQuery}
      onApply={applyDraftOperation}
      onSetNameDraft={actions.setNameDraft}
      onCommitName={onCommitName}
      onLoadLatest={save.loadLatest}
      onReset={onReset}
      onSetFocusedNodeId={setFocusedNodeId}
      t={t}
      showHeader={!embedded}
    />
  );
}

function useRefValue(value: SidebarLayout) {
  const ref = useMemo(() => ({ current: value }), []);
  ref.current = value;
  return ref;
}
