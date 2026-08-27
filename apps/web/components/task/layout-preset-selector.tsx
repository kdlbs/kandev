"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { toast } from "@/lib/toast/sonner";
import {
  IconLayout,
  IconCheck,
  IconDeviceFloppy,
  IconColumns3,
  IconFileText,
  IconDeviceDesktop,
  IconBrandVscode,
  IconRestore,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Badge } from "@kandev/ui/badge";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  BUILT_IN_LAYOUT_PROFILES,
  createLayoutProfileId,
  getBuiltInLayoutOverride,
  getLayoutProfileCompatibility,
  isBuiltInLayoutOverride,
  type BuiltInLayoutProfileId,
  type CreateLayoutProfileInput,
} from "@/lib/layout/layout-profiles";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { SavedLayout } from "@/lib/types/http";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { resolveLayoutApplySessionIds } from "./layout-preset-selector-session-ids";
import { SavedLayoutDeleteConfirmation } from "./saved-layout-delete-confirmation";
import { useSavedLayoutMutations } from "./use-saved-layout-mutations";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

type PresetOption = {
  id: BuiltInLayoutProfileId;
  label: string;
  description: string;
  icon: React.ElementType;
};

const PRESET_ICONS: Record<BuiltInLayoutProfileId, React.ElementType> = {
  default: IconColumns3,
  plan: IconFileText,
  preview: IconDeviceDesktop,
  vscode: IconBrandVscode,
};

const BUILT_IN_PRESETS: PresetOption[] = BUILT_IN_LAYOUT_PROFILES.map((profile) => ({
  id: profile.id,
  label: profile.name,
  description: profile.description,
  icon: PRESET_ICONS[profile.id],
}));

function mutationError(error: unknown): string {
  return error instanceof Error ? error.message : t("task:pleaseTryAgain");
}

function isSavedLayoutConfirmationTarget(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest("[data-confirmation-boundary]") !== null;
}

function SaveLayoutDialog({
  open,
  onOpenChange,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (input: CreateLayoutProfileInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [saving, setSaving] = useState(false);
  const captureCurrentLayout = useDockviewStore((s) => s.captureCurrentLayout);

  const handleSave = useCallback(async () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setSaving(true);
    try {
      const layout = captureCurrentLayout();
      await onSave({
        id: createLayoutProfileId(),
        name: trimmed,
        isDefault,
        layout,
        createdAt: new Date().toISOString(),
      });
      setName("");
      setIsDefault(false);
      onOpenChange(false);
    } catch (error) {
      toast.error(t("task:failedToSaveLayout"), { description: mutationError(error) });
    } finally {
      setSaving(false);
    }
  }, [name, isDefault, captureCurrentLayout, onOpenChange, onSave, t]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("task:saveCurrentLayout")}</DialogTitle>
          <DialogDescription>{t("task:saveYourCurrentPanelArrangementAs")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="layout-name">{t("task:name")}</Label>
            <Input
              id="layout-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("task:myLayout")}
              autoFocus
            />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="layout-default"
              checked={isDefault}
              onCheckedChange={(checked) => setIsDefault(checked === true)}
            />
            <Label htmlFor="layout-default" className="text-sm font-normal cursor-pointer">
              {t("task:setAsDefaultLayout")}
            </Label>
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            size="sm"
            className="cursor-pointer"
            onClick={handleSave}
            disabled={!name.trim() || saving}
          >
            {saving ? t("task:saving") : t("common:save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SavedLayoutItems({
  layouts,
  onApply,
  isFinePointer,
  confirmingDeleteId,
  onDelete,
  onCancelDelete,
  onCloseDelete,
  onConfirmDelete,
}: {
  layouts: SavedLayout[];
  onApply: (layout: SavedLayout) => void | Promise<void>;
  isFinePointer: boolean;
  confirmingDeleteId: string | null;
  onDelete: (layout: SavedLayout, anchor: HTMLElement) => void;
  onCancelDelete: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: (layoutId: string) => void | Promise<void>;
}) {
  const { t } = useTranslation();
  if (layouts.length === 0) {
    return (
      <div className="px-2 py-1.5 text-xs text-muted-foreground">{t("task:noSavedLayouts")}</div>
    );
  }
  return layouts.map((layout) => {
    const isConfirming = !isFinePointer && confirmingDeleteId === layout.id;
    if (isConfirming) {
      const title = t("task:deleteLayoutConfirm", { name: layout.name });
      const description = layout.is_default
        ? t("task:theBuiltInDefaultLayoutWill")
        : t("task:thisSavedLayoutWillBePermanently");
      return (
        <div key={layout.id} className="min-w-0" role="presentation">
          <InlineConfirmActions
            density="touch"
            testId="layout-saved-delete-inline-confirmation"
            ariaLabel={title}
            description={description}
            cancelLabel={t("common:cancel")}
            confirmLabel={t("task:delete")}
            confirmAriaLabel={t("task:delete2", { name: layout.name })}
            confirmTestId="layout-saved-delete-confirm"
            onCancel={onCancelDelete}
            onClose={onCloseDelete}
            onConfirm={() => onConfirmDelete(layout.id)}
          />
        </div>
      );
    }

    return (
      <div key={layout.id} className="flex min-w-0 items-stretch" role="presentation">
        <DropdownMenuItem
          className="min-w-0 flex-1 cursor-pointer"
          onSelect={() => void onApply(layout)}
        >
          <SavedLayoutLabel layout={layout} />
        </DropdownMenuItem>
        <DropdownMenuItem
          className="min-h-11 min-w-11 shrink-0 cursor-pointer justify-center px-2 text-destructive/60 focus:text-destructive sm:min-h-7 sm:min-w-7"
          aria-label={t("task:delete2", { name: layout.name })}
          data-testid="layout-saved-delete"
          data-layout-id={layout.id}
          onSelect={(event) => {
            event.preventDefault();
            onDelete(layout, event.currentTarget as HTMLElement);
          }}
        >
          <IconTrash className="h-3.5 w-3.5" />
        </DropdownMenuItem>
      </div>
    );
  });
}

function SavedLayoutLabel({ layout }: { layout: SavedLayout }) {
  const { t } = useTranslation();
  return (
    <>
      <IconCheck className="mr-2 h-3.5 w-3.5 shrink-0 opacity-0" />
      <span className="flex-1 truncate text-xs">{layout.name}</span>
      {layout.is_default && (
        <Badge variant="secondary" className="ml-1 px-1 py-0 text-[9px]">
          {t("common:default")}
        </Badge>
      )}
    </>
  );
}

/** Built-in preset menu items. Code-defined presets reset widths; customized
 *  built-ins restore the dimensions saved in their override. */
function BuiltInPresetItems({ onApply }: { onApply: (presetId: BuiltInLayoutProfileId) => void }) {
  return BUILT_IN_PRESETS.map((preset) => {
    const Icon = preset.icon;
    return (
      <DropdownMenuItem
        key={preset.id}
        data-testid="layout-preset-item"
        data-preset-id={preset.id}
        onClick={() => onApply(preset.id)}
        className="cursor-pointer"
      >
        <Icon className="h-4 w-4 mr-2 shrink-0" />
        <div className="flex flex-col min-w-0">
          <span className="text-xs font-medium">{preset.label}</span>
          <span className="text-[10px] text-muted-foreground truncate">{preset.description}</span>
        </div>
      </DropdownMenuItem>
    );
  });
}

function useApplySavedLayout() {
  const applyCustomLayout = useDockviewStore((s) => s.applyCustomLayout);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const appStore = useAppStoreApi();
  const { isLoaded: taskSessionsLoaded, loadSessions } = useTaskSessions(activeTaskId);

  return useCallback(
    async (layout: SavedLayout) => {
      const sessionIds = await resolveLayoutApplySessionIds({
        activeTaskId,
        activeSessionId,
        sessionsLoaded: taskSessionsLoaded,
        loadSessions,
        getSessionsForTask: (taskId) =>
          appStore.getState().taskSessionsByTask.itemsByTaskId[taskId] ?? [],
      });
      applyCustomLayout(
        {
          id: layout.id,
          name: layout.name,
          isDefault: layout.is_default,
          layout: layout.layout,
          createdAt: layout.created_at,
        },
        { activeSessionId, sessionIds },
      );
    },
    [activeSessionId, activeTaskId, appStore, applyCustomLayout, loadSessions, taskSessionsLoaded],
  );
}

function useApplyBuiltInLayout(
  savedLayouts: SavedLayout[],
  applySavedLayout: (layout: SavedLayout) => Promise<void>,
) {
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);
  return useCallback(
    (presetId: BuiltInLayoutProfileId) => {
      const override = getBuiltInLayoutOverride(savedLayouts, presetId);
      if (override && getLayoutProfileCompatibility(override).status === "editable") {
        void applySavedLayout(override);
        return;
      }
      applyBuiltInPreset(presetId, true);
    },
    [applyBuiltInPreset, applySavedLayout, savedLayouts],
  );
}

type PresetDropdownProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  deleteMenuRef: RefObject<HTMLDivElement | null>;
  tooltipOpen: boolean;
  onTooltipOpenChange: (open: boolean) => void;
  resetLayout: () => void;
  savedLayouts: SavedLayout[];
  isFinePointer: boolean;
  onApplyCustom: (layout: SavedLayout) => void | Promise<void>;
  onApplyBuiltIn: (presetId: BuiltInLayoutProfileId) => void;
  confirmingDeleteId: string | null;
  onDelete: (layout: SavedLayout, anchor: HTMLElement) => void;
  onCancelDelete: () => void;
  onCloseDelete: () => void;
  onConfirmDelete: (layoutId: string) => void | Promise<void>;
  onSaveLayout: () => void;
};

function PresetDropdown({
  open,
  onOpenChange,
  deleteMenuRef,
  tooltipOpen,
  onTooltipOpenChange,
  resetLayout,
  savedLayouts,
  isFinePointer,
  onApplyCustom,
  onApplyBuiltIn,
  confirmingDeleteId,
  onDelete,
  onCancelDelete,
  onCloseDelete,
  onConfirmDelete,
  onSaveLayout,
}: PresetDropdownProps) {
  const { t } = useTranslation();
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <Tooltip open={tooltipOpen} onOpenChange={onTooltipOpenChange}>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="cursor-pointer px-2"
              data-testid="layout-preset-trigger"
              aria-label={t("task:layoutPresets")}
            >
              <IconLayout className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{t("task:layoutPresets")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent
        ref={deleteMenuRef}
        align="end"
        className="w-60"
        onFocusOutside={(event) => {
          if (confirmingDeleteId && isSavedLayoutConfirmationTarget(event.target)) {
            event.preventDefault();
          }
        }}
        onInteractOutside={(event) => {
          if (confirmingDeleteId && isSavedLayoutConfirmationTarget(event.target)) {
            event.preventDefault();
          }
        }}
      >
        <DropdownMenuGroup>
          <DropdownMenuLabel className="text-xs">{t("task:presets")}</DropdownMenuLabel>
          <BuiltInPresetItems onApply={onApplyBuiltIn} />
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          data-testid="layout-reset-item"
          onClick={resetLayout}
          className="cursor-pointer"
        >
          <IconRestore className="h-4 w-4 mr-2 shrink-0" />
          <span className="text-xs">{t("task:resetLayout")}</span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel className="text-xs">{t("task:savedLayouts")}</DropdownMenuLabel>
          <SavedLayoutItems
            layouts={savedLayouts.filter((layout) => !isBuiltInLayoutOverride(layout))}
            onApply={onApplyCustom}
            isFinePointer={isFinePointer}
            confirmingDeleteId={confirmingDeleteId}
            onDelete={onDelete}
            onCancelDelete={onCancelDelete}
            onCloseDelete={onCloseDelete}
            onConfirmDelete={onConfirmDelete}
          />
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={onSaveLayout} className="cursor-pointer">
          <IconDeviceFloppy className="h-4 w-4 mr-2 shrink-0" />
          <span className="text-xs">{t("task:saveCurrentLayout2")}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function usePresetDropdownVisibility(onClose: () => void) {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const recentlyClosedRef = useRef(false);
  const recentlyClosedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const handleDropdownOpenChange = useCallback(
    (open: boolean) => {
      setDropdownOpen(open);
      if (!open) {
        onClose();
        recentlyClosedRef.current = true;
        setTooltipOpen(false);
        if (recentlyClosedTimerRef.current !== null) clearTimeout(recentlyClosedTimerRef.current);
        recentlyClosedTimerRef.current = setTimeout(() => {
          recentlyClosedRef.current = false;
          recentlyClosedTimerRef.current = null;
        }, 200);
      }
    },
    [onClose],
  );

  useEffect(
    () => () => {
      if (recentlyClosedTimerRef.current !== null) clearTimeout(recentlyClosedTimerRef.current);
    },
    [],
  );

  const handleTooltipOpenChange = useCallback(
    (open: boolean) => {
      if (open && (dropdownOpen || recentlyClosedRef.current)) return;
      setTooltipOpen(open);
    },
    [dropdownOpen],
  );

  return { dropdownOpen, tooltipOpen, handleDropdownOpenChange, handleTooltipOpenChange };
}

export function LayoutPresetSelector() {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [deleteCandidate, setDeleteCandidate] = useState<SavedLayout | null>(null);
  const cancelDelete = useCallback(() => setDeleteCandidate(null), []);
  const deleteAnchorRef = useRef<HTMLElement>(null);
  const deleteMenuRef = useRef<HTMLDivElement>(null);
  const resetLayout = useDockviewStore((s) => s.resetLayout);
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);
  const handleApplyCustom = useApplySavedLayout();
  const handleApplyBuiltIn = useApplyBuiltInLayout(savedLayouts, handleApplyCustom);
  const { saveLayout, deleteLayout } = useSavedLayoutMutations();
  const { dropdownOpen, tooltipOpen, handleDropdownOpenChange, handleTooltipOpenChange } =
    usePresetDropdownVisibility(cancelDelete);
  const beginDelete = useCallback((layout: SavedLayout, anchor: HTMLElement) => {
    deleteAnchorRef.current = anchor;
    setDeleteCandidate(layout);
  }, []);

  const closeDelete = useCallback(() => {
    setDeleteCandidate(null);
    handleDropdownOpenChange(false);
  }, [handleDropdownOpenChange]);

  const confirmDeleteLayout = useCallback(
    async (layoutId: string) => {
      handleDropdownOpenChange(false);
      try {
        await deleteLayout(layoutId);
      } catch (error) {
        toast.error(t("task:failedToDeleteLayout"), { description: mutationError(error) });
      }
    },
    [deleteLayout, handleDropdownOpenChange, t],
  );

  return (
    <>
      <PresetDropdown
        open={dropdownOpen}
        onOpenChange={handleDropdownOpenChange}
        deleteMenuRef={deleteMenuRef}
        tooltipOpen={tooltipOpen}
        onTooltipOpenChange={handleTooltipOpenChange}
        resetLayout={resetLayout}
        savedLayouts={savedLayouts}
        isFinePointer={isFinePointer}
        onApplyCustom={handleApplyCustom}
        onApplyBuiltIn={handleApplyBuiltIn}
        confirmingDeleteId={deleteCandidate?.id ?? null}
        onDelete={beginDelete}
        onCancelDelete={cancelDelete}
        onCloseDelete={closeDelete}
        onConfirmDelete={confirmDeleteLayout}
        onSaveLayout={() => setSaveDialogOpen(true)}
      />
      <SaveLayoutDialog
        open={saveDialogOpen}
        onOpenChange={setSaveDialogOpen}
        onSave={saveLayout}
      />
      <SavedLayoutDeleteConfirmation
        layout={deleteCandidate}
        open={isFinePointer && deleteCandidate !== null}
        anchorRef={deleteAnchorRef}
        focusBoundaryRef={deleteMenuRef}
        onOpenChange={(open) => {
          if (!open) cancelDelete();
        }}
        onConfirm={confirmDeleteLayout}
      />
    </>
  );
}
