"use client";

import { useState, type ComponentProps } from "react";
import { Button } from "@kandev/ui/button";
import { IconPlus } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import { SavedTaskViewDeleteConfirmation } from "@/components/confirmation/saved-task-view-delete-confirmation";
import { SidebarViewEditor } from "./sidebar-view-editor";
import { ViewHeaderRow } from "./view-manager";
import { useSidebarViewEditorController } from "./use-sidebar-view-editor-controller";
import { useSidebarViewPopover } from "./use-sidebar-view-popover";
import { MobileSidebarListFacetOptions } from "./mobile-sidebar-list-facet-options";
import type { MobileListFacetSettings } from "@/lib/tasks/mobile-list-query";

export function MobileSidebarViewEditor({
  workspaceId,
  facets,
}: {
  workspaceId: string | null;
  facets?: MobileListFacetSettings;
}) {
  const { t } = useTranslation();
  const [saveAsRequestId, setSaveAsRequestId] = useState<number | null>(null);
  const [isEditingName, setIsEditingName] = useState(false);
  const viewActions = useSidebarViewPopover();
  const controller = useSidebarViewEditorController({
    workspaceId,
    isDrawerLayout: true,
    deleteDensity: "touch",
    renameRequestedViewId: viewActions.renameRequestedViewId,
    onRenameRequestHandled: viewActions.consumeRenameRequest,
  });
  const deleteConfirmation = controller.deletion.target ? (
    <SavedTaskViewDeleteConfirmation
      target={controller.deletion.target}
      presentation="inline"
      open
      anchorRef={controller.deletion.anchorRef}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) controller.deletion.close();
      }}
      onConfirm={controller.deleteView}
    />
  ) : undefined;
  const headerProps: ComponentProps<typeof ViewHeaderRow> = {
    ...controller.headerProps,
    deleteConfirmation,
    draftActionsPlacement: "external",
    saveAsRequestId,
    onSaveAsRequestHandled: () => setSaveAsRequestId(null),
    onEditingChange: setIsEditingName,
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="mobile-list-view-editor">
      <div className="flex shrink-0 items-center justify-end border-b px-4 py-2">
        <Button
          type="button"
          variant="outline"
          className="h-10 cursor-pointer gap-1.5 px-3"
          onClick={() => {
            if (viewActions.newViewDisabledReason) {
              toast.info(viewActions.newViewDisabledReason);
              return;
            }
            viewActions.startNewView({ openPopover: false });
          }}
          aria-disabled={!!viewActions.newViewDisabledReason}
          data-testid="mobile-sidebar-new-view"
        >
          <IconPlus className="size-4" aria-hidden="true" />
          {t("task:newView")}
        </Button>
      </div>
      <div
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain"
        data-testid="mobile-list-view-editor-scroll"
      >
        <SidebarViewEditor {...controller.editorProps} headerProps={headerProps} />
        <MobileSidebarListFacetOptions settings={facets} />
      </div>
      {controller.hasDraft && !isEditingName && (
        <MobileViewDraftFooter
          onDiscard={controller.headerProps.onDiscard}
          onSaveAs={() => setSaveAsRequestId((current) => (current ?? 0) + 1)}
          onSave={controller.headerProps.onSaveOverwrite}
        />
      )}
    </div>
  );
}

function MobileViewDraftFooter({
  onDiscard,
  onSaveAs,
  onSave,
}: {
  onDiscard: () => void;
  onSaveAs: () => void;
  onSave: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex shrink-0 items-center justify-between gap-2 border-t bg-background px-4 pb-[max(0.75rem,env(safe-area-inset-bottom))] pt-2"
      data-testid="mobile-list-view-draft-footer"
    >
      <Button
        type="button"
        variant="ghost"
        className="h-10 cursor-pointer px-3"
        onClick={onDiscard}
        data-testid="view-discard-button"
      >
        {t("task:discard")}
      </Button>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          className="h-10 cursor-pointer px-3"
          onClick={onSaveAs}
          data-testid="view-save-as-button"
        >
          {t("task:saveAs2")}
        </Button>
        <Button
          type="button"
          className="h-10 cursor-pointer px-3"
          onClick={onSave}
          data-testid="view-save-button"
        >
          {t("common:save")}
        </Button>
      </div>
    </div>
  );
}
