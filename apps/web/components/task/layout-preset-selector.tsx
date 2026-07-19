"use client";

import { useCallback, useRef, useState } from "react";
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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
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
import { useDockviewStore, type BuiltInPreset } from "@/lib/state/dockview-store";
import {
  BUILT_IN_LAYOUT_PROFILES,
  createLayoutProfile,
  createLayoutProfileId,
  deleteLayoutProfile,
  type BuiltInLayoutProfileId,
} from "@/lib/layout/layout-profiles";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { SavedLayout } from "@/lib/types/http";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { resolveLayoutApplySessionIds } from "./layout-preset-selector-session-ids";

type PresetOption = {
  id: BuiltInPreset;
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

function SaveLayoutDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [name, setName] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [saving, setSaving] = useState(false);
  const captureCurrentLayout = useDockviewStore((s) => s.captureCurrentLayout);
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);
  const setUserSettings = useAppStore((s) => s.setUserSettings);

  const handleSave = useCallback(async () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setSaving(true);
    try {
      const layout = captureCurrentLayout();
      const response = await updateUserSettings({
        saved_layouts: createLayoutProfile(savedLayouts, {
          id: createLayoutProfileId(),
          name: trimmed,
          isDefault,
          layout,
          createdAt: new Date().toISOString(),
        }),
      });
      setUserSettings(mapUserSettingsResponse(response));
      setName("");
      setIsDefault(false);
      onOpenChange(false);
    } finally {
      setSaving(false);
    }
  }, [name, isDefault, captureCurrentLayout, savedLayouts, setUserSettings, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Save Current Layout</DialogTitle>
          <DialogDescription>
            Save your current panel arrangement as a reusable layout.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="layout-name">Name</Label>
            <Input
              id="layout-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My layout"
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
              Set as default layout
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
            Cancel
          </Button>
          <Button
            size="sm"
            className="cursor-pointer"
            onClick={handleSave}
            disabled={!name.trim() || saving}
          >
            {saving ? "Saving..." : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SavedLayoutItems({
  layouts,
  onApply,
  onDelete,
}: {
  layouts: SavedLayout[];
  onApply: (layout: SavedLayout) => void | Promise<void>;
  onDelete: (layout: SavedLayout) => void;
}) {
  if (layouts.length === 0) {
    return <div className="px-2 py-1.5 text-xs text-muted-foreground">No saved layouts</div>;
  }
  return layouts.map((layout) => (
    <div key={layout.id} className="flex items-stretch" role="presentation">
      <DropdownMenuItem
        className="min-w-0 flex-1 cursor-pointer"
        onSelect={() => void onApply(layout)}
      >
        <IconCheck className="mr-2 h-3.5 w-3.5 shrink-0 opacity-0" />
        <span className="flex-1 truncate text-xs">{layout.name}</span>
        {layout.is_default && (
          <Badge variant="secondary" className="ml-1 px-1 py-0 text-[9px]">
            default
          </Badge>
        )}
      </DropdownMenuItem>
      <DropdownMenuItem
        className="min-h-11 min-w-11 shrink-0 cursor-pointer justify-center px-2 text-destructive/60 focus:text-destructive sm:min-h-7 sm:min-w-7"
        aria-label={`Delete ${layout.name}`}
        data-testid="layout-saved-delete"
        data-layout-id={layout.id}
        onSelect={() => onDelete(layout)}
      >
        <IconTrash className="h-3.5 w-3.5" />
      </DropdownMenuItem>
    </div>
  ));
}

/** The built-in preset menu items. Applying any preset resets widths
 *  (resetWidths=true), which is how a user clears a custom sidebar width. */
function BuiltInPresetItems({ onApply }: { onApply: (presetId: BuiltInPreset) => void }) {
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

function DeleteLayoutDialog({
  layout,
  onClose,
  onDelete,
}: {
  layout: SavedLayout | null;
  onClose: () => void;
  onDelete: (layoutId: string) => void;
}) {
  return (
    <AlertDialog open={Boolean(layout)} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {layout?.name ?? "saved layout"}?</AlertDialogTitle>
          <AlertDialogDescription>
            {layout?.is_default
              ? "The built-in Default layout will become the default."
              : "This saved layout will be permanently removed."}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            onClick={() => {
              if (layout) onDelete(layout.id);
              onClose();
            }}
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function LayoutPresetSelector() {
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const [deleteCandidate, setDeleteCandidate] = useState<SavedLayout | null>(null);
  const recentlyClosedRef = useRef(false);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);
  const resetLayout = useDockviewStore((s) => s.resetLayout);
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);
  const setUserSettings = useAppStore((s) => s.setUserSettings);
  const handleApplyCustom = useApplySavedLayout();

  const handleDeleteLayout = useCallback(
    async (layoutId: string) => {
      const response = await updateUserSettings({
        saved_layouts: deleteLayoutProfile(savedLayouts, layoutId),
      });
      setUserSettings(mapUserSettingsResponse(response));
    },
    [savedLayouts, setUserSettings],
  );

  const handleDropdownOpenChange = useCallback((open: boolean) => {
    setDropdownOpen(open);
    if (!open) {
      recentlyClosedRef.current = true;
      setTooltipOpen(false);
      setTimeout(() => {
        recentlyClosedRef.current = false;
      }, 200);
    }
  }, []);

  const handleTooltipOpenChange = useCallback(
    (open: boolean) => {
      if (open && (dropdownOpen || recentlyClosedRef.current)) return;
      setTooltipOpen(open);
    },
    [dropdownOpen],
  );

  return (
    <>
      <DropdownMenu open={dropdownOpen} onOpenChange={handleDropdownOpenChange}>
        <Tooltip open={tooltipOpen} onOpenChange={handleTooltipOpenChange}>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className="cursor-pointer px-2"
                data-testid="layout-preset-trigger"
              >
                <IconLayout className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent side="bottom">Layout presets</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="text-xs">Presets</DropdownMenuLabel>
            <BuiltInPresetItems onApply={(id) => applyBuiltInPreset(id, true)} />
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            data-testid="layout-reset-item"
            onClick={resetLayout}
            className="cursor-pointer"
          >
            <IconRestore className="h-4 w-4 mr-2 shrink-0" />
            <span className="text-xs">Reset Layout</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            <DropdownMenuLabel className="text-xs">Saved Layouts</DropdownMenuLabel>
            <SavedLayoutItems
              layouts={savedLayouts}
              onApply={handleApplyCustom}
              onDelete={setDeleteCandidate}
            />
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => setSaveDialogOpen(true)} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-2 shrink-0" />
            <span className="text-xs">Save current layout...</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <SaveLayoutDialog open={saveDialogOpen} onOpenChange={setSaveDialogOpen} />
      <DeleteLayoutDialog
        layout={deleteCandidate}
        onClose={() => setDeleteCandidate(null)}
        onDelete={(layoutId) => void handleDeleteLayout(layoutId)}
      />
    </>
  );
}
