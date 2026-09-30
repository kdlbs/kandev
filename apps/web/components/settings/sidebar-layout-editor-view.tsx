"use client";

import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconRefresh } from "@tabler/icons-react";
import { SettingsPageHeader } from "@/components/settings/settings-typography";
import type { SidebarLayout } from "@/lib/sidebar/layout-types";
import type {
  ProjectedSidebarNode,
  SidebarLayoutProjection,
} from "@/lib/sidebar/layout-projection";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { DraftOperation } from "./sidebar-layout-editor-types";
import { SidebarDraftPreview } from "./sidebar-layout-editor-preview";
import { SidebarLayoutNodesCard } from "./sidebar-layout-editor-node-list";
import { FocusedSidebarGroup } from "./sidebar-layout-editor-focused-group";

const SIDEBAR_LABEL_KEY = "settings:sidebar";

export type SidebarLayoutContentProps = {
  draft: SidebarLayout;
  readOnly: boolean;
  projected: SidebarLayoutProjection;
  visibleNodes: ProjectedSidebarNode[];
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
  onReset: () => void;
  onSetFocusedNodeId: (value: string | null) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
  isMobile: boolean;
  showHeader?: boolean;
};

export function SidebarLayoutContent({
  draft,
  readOnly,
  projected,
  visibleNodes,
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
  onReset,
  onSetFocusedNodeId,
  t,
  isMobile,
  showHeader = true,
}: SidebarLayoutContentProps) {
  return (
    <div className="min-w-0 space-y-6" data-testid="sidebar-layout-editor" data-mobile={isMobile}>
      {showHeader && (
        <>
          <SettingsPageHeader
            title={t(SIDEBAR_LABEL_KEY)}
            description={t("settings:sidebarDescription")}
          />
          <Separator />
        </>
      )}
      <div className="grid min-w-0 gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,0.8fr)]">
        <SidebarLayoutNodesCard
          draft={draft}
          readOnly={readOnly}
          projected={projected}
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
          validationMessage={validationMessage}
          operationError={operationError}
          saveError={saveError}
          latestLoading={latestLoading}
          onLoadLatest={onLoadLatest}
          onApply={onApply}
          onSetNameDraft={onSetNameDraft}
          onCommitName={onCommitName}
          onSetFocusedNodeId={onSetFocusedNodeId}
          t={t}
        />
        <SidebarDraftPreview nodes={visibleNodes} t={t} />
      </div>
      <Button
        type="button"
        variant="outline"
        className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={onReset}
        disabled={readOnly}
      >
        <IconRefresh className="mr-2 h-4 w-4" />
        {t("settings:sidebarRestoreDefaults")}
      </Button>
    </div>
  );
}

export { FocusedSidebarGroup };
