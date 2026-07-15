"use client";

import { createElement, useCallback, useMemo, useState } from "react";
import { IconPlus, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useJiraTaskPresets } from "@/components/jira/my-jira/use-task-presets";
import {
  DEFAULT_JIRA_PRESETS,
  PRESET_ICON_CHOICES,
  iconForPresetKey,
  type JiraStoredPreset,
} from "@/components/jira/my-jira/presets";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

const JIRA_PROMPT_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "key",
    description: "Jira ticket key",
    example: "PROJ-123",
    executor_types: [],
  },
  {
    key: "url",
    description: "URL of the Jira ticket",
    example: "https://company.atlassian.net/browse/PROJ-123",
    executor_types: [],
  },
  {
    key: "title",
    description: "Ticket summary",
    example: "Login button broken on Safari",
    executor_types: [],
  },
  {
    key: "description",
    description: "Ticket description",
    example: "Repro: open Safari, click login…",
    executor_types: [],
  },
];

function newPreset(): JiraStoredPreset {
  return {
    id: `preset_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New action",
    hint: "",
    icon: "sparkle",
    prompt_template: "",
  };
}

function IconSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
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
  preset: JiraStoredPreset;
  expanded: boolean;
  onToggle: () => void;
  onPatch: (patch: Partial<JiraStoredPreset>) => void;
  onRemove: () => void;
}) {
  return (
    <div className="rounded-md border">
      <div className="flex items-end gap-2 p-2">
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">Icon</span>
          <IconSelect value={preset.icon} onChange={(v) => onPatch({ icon: v })} />
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
        <Button
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer text-xs"
          onClick={onToggle}
        >
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
              placeholders={JIRA_PROMPT_PLACEHOLDERS}
            />
          </div>
          <p className="text-[11px] text-muted-foreground/60">
            Type {"{{"} to see available placeholders.{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{key}}"}</code>,{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{url}}"}</code>,{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{title}}"}</code>, and{" "}
            <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{description}}"}</code>{" "}
            are substituted when the action runs.
          </p>
        </div>
      )}
    </div>
  );
}

function usePresetDraft() {
  const { stored, save: persistSave, loaded } = useJiraTaskPresets();
  const [draft, setDraft] = useState<JiraStoredPreset[]>(stored);
  // Render-time conditional setState is React's documented "adjust state
  // during render" pattern; it resets the draft when the hook's stored value
  // changes (e.g. after reset or a backend refresh). Gate the sync on `loaded`
  // so an in-progress edit isn't wiped when the initial settings read lands.
  const [baseline, setBaseline] = useState(stored);
  const dirty = useMemo(
    () => JSON.stringify(baseline) !== JSON.stringify(draft),
    [baseline, draft],
  );
  const [synced, setSynced] = useState(stored);
  if (loaded && stored !== synced && !dirty) {
    setSynced(stored);
    setBaseline(stored);
    setDraft(stored);
  }
  const save = useCallback(async () => {
    const submitted = draft;
    await persistSave(submitted);
    setBaseline(submitted);
    setDraft((current) =>
      JSON.stringify(current) === JSON.stringify(submitted) ? submitted : current,
    );
  }, [persistSave, draft]);
  const reset = useCallback(() => {
    setDraft(DEFAULT_JIRA_PRESETS);
  }, []);
  const discard = useCallback(() => setDraft(baseline), [baseline]);
  return { draft, setDraft, dirty, save, reset, discard, loaded };
}

export function TaskPresetsSection() {
  const { toast } = useToast();
  const { draft, setDraft, dirty, save, reset, discard, loaded } = usePresetDraft();
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const patch = useCallback(
    (index: number, change: Partial<JiraStoredPreset>) => {
      setDraft(draft.map((p, i) => (i === index ? { ...p, ...change } : p)));
    },
    [draft, setDraft],
  );
  const remove = useCallback(
    (index: number) => setDraft(draft.filter((_, i) => i !== index)),
    [draft, setDraft],
  );
  const add = useCallback(() => {
    const created = newPreset();
    setDraft([...draft, created]);
    setExpandedId(created.id);
  }, [draft, setDraft]);

  const handleSave = useCallback(async () => {
    try {
      await save();
      toast({ description: "Task presets saved", variant: "success" });
    } catch {
      toast({ description: "Failed to save task presets", variant: "error" });
      throw new Error("Failed to save task presets");
    }
  }, [save, toast]);

  useSettingsSaveContributor({
    id: "jira-task-presets",
    revision: JSON.stringify(draft),
    isDirty: dirty,
    canSave: loaded,
    save: handleSave,
    discard,
  });

  return (
    <SettingsSection
      title="Task presets"
      description="Prompts shown on /jira when starting a task from a ticket."
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={reset}
            disabled={!loaded}
            className="cursor-pointer"
          >
            <IconRefresh className="h-3.5 w-3.5 mr-1" />
            Reset
          </Button>
        </div>
      }
    >
      <div className="space-y-2">
        {draft.map((preset, index) => (
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
          Add preset
        </Button>
      </div>
    </SettingsSection>
  );
}
