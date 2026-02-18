'use client';

import { useCallback, useState } from 'react';
import {
  IconLayout,
  IconCheck,
  IconDeviceFloppy,
  IconColumns3,
  IconFileText,
  IconDeviceDesktop,
  IconTrash,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Checkbox } from '@kandev/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Badge } from '@kandev/ui/badge';
import { useDockviewStore, type BuiltInPreset } from '@/lib/state/dockview-store';
import { useAppStore } from '@/components/state-provider';
import { updateUserSettings } from '@/lib/api/domains/settings-api';
import type { SavedLayout } from '@/lib/types/http';

type PresetOption = {
  id: BuiltInPreset;
  label: string;
  description: string;
  icon: React.ElementType;
};

const BUILT_IN_PRESETS: PresetOption[] = [
  {
    id: 'default',
    label: 'Default',
    description: 'Sidebar + Agent + Changes/Files/Terminal',
    icon: IconColumns3,
  },
  {
    id: 'plan',
    label: 'Plan Mode',
    description: 'Sidebar + Agent + Plan (side-by-side)',
    icon: IconFileText,
  },
  {
    id: 'preview',
    label: 'Preview Mode',
    description: 'Sidebar + Agent + Browser (side-by-side)',
    icon: IconDeviceDesktop,
  },
];

function SaveLayoutDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [name, setName] = useState('');
  const [isDefault, setIsDefault] = useState(false);
  const [saving, setSaving] = useState(false);
  const captureCurrentLayout = useDockviewStore((s) => s.captureCurrentLayout);
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);

  const handleSave = useCallback(async () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setSaving(true);
    try {
      const layout = captureCurrentLayout();
      const newLayout: SavedLayout = {
        id: `layout-${Date.now()}`,
        name: trimmed,
        is_default: isDefault,
        layout,
        created_at: new Date().toISOString(),
      };
      // If this layout is the default, unset other defaults
      const updated = savedLayouts.map((l) =>
        isDefault ? { ...l, is_default: false } : l
      );
      await updateUserSettings({
        saved_layouts: [...updated, newLayout],
      });
      setName('');
      setIsDefault(false);
      onOpenChange(false);
    } finally {
      setSaving(false);
    }
  }, [name, isDefault, captureCurrentLayout, savedLayouts, onOpenChange]);

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
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSave();
              }}
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
            {saving ? 'Saving...' : 'Save'}
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
  onApply: (layout: SavedLayout) => void;
  onDelete: (layoutId: string) => void;
}) {
  if (layouts.length === 0) {
    return (
      <div className="px-2 py-1.5 text-xs text-muted-foreground">
        No saved layouts
      </div>
    );
  }
  return layouts.map((layout) => (
    <DropdownMenuItem
      key={layout.id}
      className="cursor-pointer group/layout"
      onClick={() => onApply(layout)}
    >
      <IconCheck className="h-3.5 w-3.5 mr-2 shrink-0 opacity-0" />
      <span className="text-xs truncate flex-1">{layout.name}</span>
      {layout.is_default && (
        <Badge variant="secondary" className="text-[9px] px-1 py-0 ml-1">
          default
        </Badge>
      )}
      <button
        type="button"
        className="ml-1 text-destructive/60 hover:text-destructive opacity-0 group-hover/layout:opacity-100 transition-opacity cursor-pointer"
        onClick={(e) => {
          e.stopPropagation();
          onDelete(layout.id);
        }}
      >
        <IconTrash className="h-3.5 w-3.5" />
      </button>
    </DropdownMenuItem>
  ));
}

export function LayoutPresetSelector() {
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);
  const applyCustomLayout = useDockviewStore((s) => s.applyCustomLayout);
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);

  const handleApplyCustom = useCallback(
    (layout: SavedLayout) => applyCustomLayout({
      id: layout.id,
      name: layout.name,
      isDefault: layout.is_default,
      layout: layout.layout,
      createdAt: layout.created_at,
    }),
    [applyCustomLayout]
  );

  const handleDeleteLayout = useCallback(
    async (layoutId: string) => {
      await updateUserSettings({ saved_layouts: savedLayouts.filter((l) => l.id !== layoutId) });
    },
    [savedLayouts]
  );

  return (
    <>
      <DropdownMenu open={dropdownOpen} onOpenChange={setDropdownOpen}>
        <Tooltip open={dropdownOpen ? false : tooltipOpen} onOpenChange={setTooltipOpen}>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" className="cursor-pointer px-2">
                <IconLayout className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent side="bottom">Layout presets</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="text-xs">Presets</DropdownMenuLabel>
            {BUILT_IN_PRESETS.map((preset) => {
              const Icon = preset.icon;
              return (
                <DropdownMenuItem key={preset.id} onClick={() => applyBuiltInPreset(preset.id)} className="cursor-pointer">
                  <Icon className="h-4 w-4 mr-2 shrink-0" />
                  <div className="flex flex-col min-w-0">
                    <span className="text-xs font-medium">{preset.label}</span>
                    <span className="text-[10px] text-muted-foreground truncate">{preset.description}</span>
                  </div>
                </DropdownMenuItem>
              );
            })}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            <DropdownMenuLabel className="text-xs">Saved Layouts</DropdownMenuLabel>
            <SavedLayoutItems layouts={savedLayouts} onApply={handleApplyCustom} onDelete={handleDeleteLayout} />
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => setSaveDialogOpen(true)} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-2 shrink-0" />
            <span className="text-xs">Save current layout...</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <SaveLayoutDialog open={saveDialogOpen} onOpenChange={setSaveDialogOpen} />
    </>
  );
}
