"use client";

import { useMemo, useState, useCallback } from "react";
import { IconArrowDown, IconArrowUp, IconPlus, IconTrash, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useGitHubActionPresets } from "@/hooks/domains/github/use-github-action-presets";
import {
  DEFAULT_ISSUE_PRESETS,
  DEFAULT_PR_PRESETS,
  PRESET_ICON_CHOICES,
} from "@/components/github/my-github/action-presets";
import type { GitHubActionPreset } from "@/lib/types/github";

type PresetKind = "pr" | "issue";

type EditorProps = {
  kind: PresetKind;
  presets: GitHubActionPreset[];
  onChange: (presets: GitHubActionPreset[]) => void;
};

function newPreset(): GitHubActionPreset {
  return {
    id: `preset_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New action",
    hint: "",
    icon: "sparkle",
    prompt_template: "",
  };
}

function PresetIconSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 cursor-pointer">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {PRESET_ICON_CHOICES.map((choice) => {
          const ChoiceIcon = choice.icon;
          return (
            <SelectItem key={choice.key} value={choice.key} className="cursor-pointer">
              <span className="flex items-center gap-2">
                <ChoiceIcon className="h-3.5 w-3.5" />
                {choice.label}
              </span>
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}

function PresetRowControls({
  index,
  total,
  onMove,
  onRemove,
}: {
  index: number;
  total: number;
  onMove: (direction: -1 | 1) => void;
  onRemove: () => void;
}) {
  return (
    <div className="flex flex-col gap-1 pt-[22px]">
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7 cursor-pointer"
        disabled={index === 0}
        onClick={() => onMove(-1)}
        aria-label="Move up"
      >
        <IconArrowUp className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7 cursor-pointer"
        disabled={index === total - 1}
        onClick={() => onMove(1)}
        aria-label="Move down"
      >
        <IconArrowDown className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7 cursor-pointer text-destructive"
        onClick={onRemove}
        aria-label="Remove"
      >
        <IconTrash className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function PresetRow({
  preset,
  index,
  total,
  onPatch,
  onMove,
  onRemove,
}: {
  preset: GitHubActionPreset;
  index: number;
  total: number;
  onPatch: (patch: Partial<GitHubActionPreset>) => void;
  onMove: (direction: -1 | 1) => void;
  onRemove: () => void;
}) {
  return (
    <div className="rounded-md border p-3 space-y-2">
      <div className="flex items-start gap-2">
        <div className="flex-1 grid grid-cols-[auto_1fr_1fr] gap-2 items-start">
          <div className="min-w-[6.5rem]">
            <Label className="text-xs mb-1 block">Icon</Label>
            <PresetIconSelect value={preset.icon} onChange={(v) => onPatch({ icon: v })} />
          </div>
          <div>
            <Label className="text-xs mb-1 block">Label</Label>
            <Input
              className="h-8"
              value={preset.label}
              onChange={(e) => onPatch({ label: e.target.value })}
            />
          </div>
          <div>
            <Label className="text-xs mb-1 block">Hint</Label>
            <Input
              className="h-8"
              value={preset.hint}
              onChange={(e) => onPatch({ hint: e.target.value })}
            />
          </div>
        </div>
        <PresetRowControls index={index} total={total} onMove={onMove} onRemove={onRemove} />
      </div>
      <div>
        <Label className="text-xs mb-1 block">
          Prompt template — use <code className="text-[10px]">{"{url}"}</code> and{" "}
          <code className="text-[10px]">{"{title}"}</code>
        </Label>
        <Textarea
          rows={2}
          className="text-xs font-mono"
          value={preset.prompt_template}
          onChange={(e) => onPatch({ prompt_template: e.target.value })}
        />
      </div>
    </div>
  );
}

function PresetEditor({ kind, presets, onChange }: EditorProps) {
  const patch = useCallback(
    (index: number, change: Partial<GitHubActionPreset>) => {
      onChange(presets.map((p, i) => (i === index ? { ...p, ...change } : p)));
    },
    [presets, onChange],
  );
  const move = useCallback(
    (index: number, direction: -1 | 1) => {
      const target = index + direction;
      if (target < 0 || target >= presets.length) return;
      const next = [...presets];
      const [item] = next.splice(index, 1);
      next.splice(target, 0, item);
      onChange(next);
    },
    [presets, onChange],
  );
  const remove = useCallback(
    (index: number) => {
      onChange(presets.filter((_, i) => i !== index));
    },
    [presets, onChange],
  );
  const add = useCallback(() => {
    onChange([...presets, newPreset()]);
  }, [presets, onChange]);

  return (
    <div className="space-y-3">
      {presets.map((preset, index) => (
        <PresetRow
          key={preset.id}
          preset={preset}
          index={index}
          total={presets.length}
          onPatch={(patchObj) => patch(index, patchObj)}
          onMove={(direction) => move(index, direction)}
          onRemove={() => remove(index)}
        />
      ))}
      <Button size="sm" variant="outline" onClick={add} className="cursor-pointer">
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        Add {kind === "pr" ? "PR action" : "issue action"}
      </Button>
    </div>
  );
}

function usePresetDrafts(workspaceId: string): {
  prDraft: GitHubActionPreset[];
  issueDraft: GitHubActionPreset[];
  setPrDraft: (next: GitHubActionPreset[]) => void;
  setIssueDraft: (next: GitHubActionPreset[]) => void;
  dirty: boolean;
  save: () => Promise<void>;
  reset: () => Promise<void>;
  loading: boolean;
} {
  const { presets, save, reset, loading } = useGitHubActionPresets(workspaceId);
  const [prDraft, setPrDraft] = useState<GitHubActionPreset[]>(DEFAULT_PR_PRESETS);
  const [issueDraft, setIssueDraft] = useState<GitHubActionPreset[]>(DEFAULT_ISSUE_PRESETS);
  // When a new `presets` object arrives from the server, reset the editor drafts.
  // Render-time conditional setState is React's documented pattern for deriving
  // state from props with a reset; preferred over useEffect for sync updates.
  const [syncedPresets, setSyncedPresets] = useState(presets);
  if (presets && presets !== syncedPresets) {
    setSyncedPresets(presets);
    setPrDraft(presets.pr?.length ? presets.pr : DEFAULT_PR_PRESETS);
    setIssueDraft(presets.issue?.length ? presets.issue : DEFAULT_ISSUE_PRESETS);
  }

  const dirty = useMemo(() => {
    const currentPR = presets?.pr ?? DEFAULT_PR_PRESETS;
    const currentIssue = presets?.issue ?? DEFAULT_ISSUE_PRESETS;
    return (
      JSON.stringify(currentPR) !== JSON.stringify(prDraft) ||
      JSON.stringify(currentIssue) !== JSON.stringify(issueDraft)
    );
  }, [presets, prDraft, issueDraft]);

  const persist = useCallback(async () => {
    await save({ pr: prDraft, issue: issueDraft });
  }, [save, prDraft, issueDraft]);

  const doReset = useCallback(async () => {
    const response = await reset();
    if (response) {
      setPrDraft(response.pr?.length ? response.pr : DEFAULT_PR_PRESETS);
      setIssueDraft(response.issue?.length ? response.issue : DEFAULT_ISSUE_PRESETS);
    }
  }, [reset]);

  return {
    prDraft,
    issueDraft,
    setPrDraft,
    setIssueDraft,
    dirty,
    save: persist,
    reset: doReset,
    loading,
  };
}

export function ActionPresetsSection({ workspaceId }: { workspaceId: string }) {
  const { toast } = useToast();
  const { prDraft, issueDraft, setPrDraft, setIssueDraft, dirty, save, reset, loading } =
    usePresetDrafts(workspaceId);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await save();
      toast({ description: "Quick actions saved", variant: "success" });
    } catch {
      toast({ description: "Failed to save quick actions", variant: "error" });
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    try {
      await reset();
      toast({ description: "Quick actions reset to defaults", variant: "success" });
    } catch {
      toast({ description: "Failed to reset quick actions", variant: "error" });
    }
  };

  return (
    <SettingsSection
      title="Quick actions"
      description="Prompts shown on the /github page when starting a task from a PR or issue."
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={handleReset}
            disabled={loading || saving}
            className="cursor-pointer"
          >
            <IconRefresh className="h-3.5 w-3.5 mr-1" />
            Reset
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={!dirty || saving}
            className="cursor-pointer"
          >
            Save changes
          </Button>
        </div>
      }
    >
      <Card>
        <CardContent className="p-4 space-y-5">
          <div>
            <div className="text-sm font-semibold mb-2">Pull request actions</div>
            <PresetEditor kind="pr" presets={prDraft} onChange={setPrDraft} />
          </div>
          <div>
            <div className="text-sm font-semibold mb-2">Issue actions</div>
            <PresetEditor kind="issue" presets={issueDraft} onChange={setIssueDraft} />
          </div>
        </CardContent>
      </Card>
    </SettingsSection>
  );
}
