"use client";

import { createElement, useMemo, useState, useCallback } from "react";
import { IconPlus, IconTrash, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@kandev/ui/tabs";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useGitHubActionPresets } from "@/hooks/domains/github/use-github-action-presets";
import {
  DEFAULT_ISSUE_PRESETS,
  DEFAULT_PR_PRESETS,
  PRESET_ICON_CHOICES,
  iconForPresetKey,
} from "@/components/github/my-github/action-presets";
import type { GitHubActionPreset } from "@/lib/types/github";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

const ACTION_PROMPT_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "url",
    description: "URL of the PR or issue",
    example: "https://github.com/org/repo/pull/42",
    executor_types: [],
  },
  {
    key: "title",
    description: "Title of the PR or issue",
    example: "Fix login page crash",
    executor_types: [],
  },
];

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
      <SelectTrigger className="!h-8 py-0.5 text-sm cursor-pointer" aria-label="Icon">
        <SelectValue>
          {createElement(iconForPresetKey(value), { className: "h-4 w-4" })}
        </SelectValue>
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

function PresetRow({
  preset,
  expanded,
  onToggle,
  onPatch,
  onRemove,
}: {
  preset: GitHubActionPreset;
  expanded: boolean;
  onToggle: () => void;
  onPatch: (patch: Partial<GitHubActionPreset>) => void;
  onRemove: () => void;
}) {
  return (
    <div className="rounded-md border">
      <div className="flex items-end gap-2 p-2">
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">Icon</span>
          <PresetIconSelect value={preset.icon} onChange={(v) => onPatch({ icon: v })} />
        </div>
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">Label</span>
          <Input
            className="h-8 w-40"
            value={preset.label}
            placeholder="Label"
            onChange={(e) => onPatch({ label: e.target.value })}
          />
        </div>
        <div className="flex flex-col gap-0.5 flex-1">
          <span className="text-[10px] text-muted-foreground">Hint</span>
          <Input
            className="h-8"
            value={preset.hint}
            placeholder="Hint (optional)"
            onChange={(e) => onPatch({ hint: e.target.value })}
          />
        </div>
        <Button variant="outline" size="sm" className="h-8 cursor-pointer text-xs" onClick={onToggle}>
          {expanded ? "Hide prompt" : "Edit prompt"}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer text-destructive"
          onClick={onRemove}
          aria-label="Remove"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
      {expanded && (
        <div className="px-2 pb-2 space-y-1">
          <div className="rounded-md border overflow-hidden">
            <ScriptEditor
              value={preset.prompt_template}
              onChange={(v) => onPatch({ prompt_template: v })}
              language="markdown"
              height={computeEditorHeight(preset.prompt_template)}
              lineNumbers="off"
              placeholders={ACTION_PROMPT_PLACEHOLDERS}
            />
          </div>
          <p className="text-[11px] text-muted-foreground/60">
            Type {"{{"} to see available placeholders.{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{url}}"}</code> and{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{title}}"}</code> are
            substituted when the action runs.
          </p>
        </div>
      )}
    </div>
  );
}

function PresetEditor({
  presets,
  onChange,
  addLabel,
}: {
  presets: GitHubActionPreset[];
  onChange: (presets: GitHubActionPreset[]) => void;
  addLabel: string;
}) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const patch = useCallback(
    (index: number, change: Partial<GitHubActionPreset>) => {
      onChange(presets.map((p, i) => (i === index ? { ...p, ...change } : p)));
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
    const created = newPreset();
    onChange([...presets, created]);
    setExpandedId(created.id);
  }, [presets, onChange]);

  return (
    <div className="space-y-2">
      {presets.map((preset, index) => (
        <PresetRow
          key={preset.id}
          preset={preset}
          expanded={expandedId === preset.id}
          onToggle={() => setExpandedId((id) => (id === preset.id ? null : preset.id))}
          onPatch={(p) => patch(index, p)}
          onRemove={() => remove(index)}
        />
      ))}
      <Button size="sm" variant="outline" onClick={add} className="cursor-pointer">
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        {addLabel}
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
  const [prDraft, setPrDraft] = useState<GitHubActionPreset[]>(() =>
    presets?.pr?.length ? presets.pr : DEFAULT_PR_PRESETS,
  );
  const [issueDraft, setIssueDraft] = useState<GitHubActionPreset[]>(() =>
    presets?.issue?.length ? presets.issue : DEFAULT_ISSUE_PRESETS,
  );
  // Render-time conditional setState is React's documented "adjust state
  // during render" pattern; it resets drafts whenever a new server response
  // replaces the presets reference.
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
  const [resetting, setResetting] = useState(false);

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
    setResetting(true);
    try {
      await reset();
      toast({ description: "Quick actions reset to defaults", variant: "success" });
    } catch {
      toast({ description: "Failed to reset quick actions", variant: "error" });
    } finally {
      setResetting(false);
    }
  };

  return (
    <SettingsSection
      title="Quick actions"
      description="Prompts shown on /github when starting a task from a PR or issue."
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={handleReset}
            disabled={loading || saving || resetting}
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
      <Tabs defaultValue="pr">
        <TabsList>
          <TabsTrigger value="pr" className="cursor-pointer">
            Pull requests
          </TabsTrigger>
          <TabsTrigger value="issue" className="cursor-pointer">
            Issues
          </TabsTrigger>
        </TabsList>
        <TabsContent value="pr">
          <PresetEditor presets={prDraft} onChange={setPrDraft} addLabel="Add PR action" />
        </TabsContent>
        <TabsContent value="issue">
          <PresetEditor presets={issueDraft} onChange={setIssueDraft} addLabel="Add issue action" />
        </TabsContent>
      </Tabs>
    </SettingsSection>
  );
}
