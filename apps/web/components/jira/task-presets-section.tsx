"use client";

import { createElement, useCallback, useMemo, useState } from "react";
import { IconPlus, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
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
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

// A function rather than a const because `description` is copy — it is shown in
// the editor's completion list — and a module-scope `t()` would freeze it at the
// boot locale. `key` is the interpolation token and each `example` is sample
// Jira data; both are identifiers and stay out of the catalog.
function jiraPromptPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "key",
      description: t("jira:presetPlaceholderTicketKey"),
      example: "PROJ-123",
      executor_types: [],
    },
    {
      key: "url",
      description: t("jira:presetPlaceholderTicketUrl"),
      example: "https://company.atlassian.net/browse/PROJ-123",
      executor_types: [],
    },
    {
      key: "title",
      description: t("jira:presetPlaceholderTicketSummary"),
      example: "Login button broken on Safari",
      executor_types: [],
    },
    {
      key: "description",
      description: t("jira:presetPlaceholderTicketDescription"),
      example: "Repro: open Safari, click login…",
      executor_types: [],
    },
  ];
}

// The My Jira page these presets appear on. A route, not copy.
const MY_JIRA_ROUTE = "/jira";

// `label` is PERSISTED to user settings as part of `JiraStoredPreset`, and the
// row below is immediately editable, so it must stay locale-neutral: a preset
// seeded in one locale and saved unedited would keep that locale's text forever.
// Same contract as `newPreset` in components/github/action-presets-section.tsx.
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
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="!h-8 py-0.5 text-sm cursor-pointer" aria-label={t("jira:icon")}>
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
                {t(choice.labelKey)}
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
  savedPreset,
  expanded,
  onToggle,
  onPatch,
  onRemove,
}: {
  preset: JiraStoredPreset;
  savedPreset?: JiraStoredPreset;
  expanded: boolean;
  onToggle: () => void;
  onPatch: (patch: Partial<JiraStoredPreset>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const fieldIsDirty = <K extends keyof JiraStoredPreset>(field: K) =>
    !savedPreset || preset[field] !== savedPreset[field];
  const isDirty = !savedPreset || JSON.stringify(preset) !== JSON.stringify(savedPreset);

  return (
    <div
      className="rounded-md border"
      data-settings-dirty={isDirty}
      data-testid={`jira-task-preset-${preset.id}`}
    >
      <div className="flex items-end gap-2 p-2">
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">{t("jira:icon")}</span>
          <div
            data-settings-dirty={fieldIsDirty("icon")}
            className="rounded-md border border-transparent"
          >
            <IconSelect value={preset.icon} onChange={(v) => onPatch({ icon: v })} />
          </div>
        </div>
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">{t("jira:label")}</span>
          <Input
            className="h-8 w-40"
            value={preset.label}
            data-settings-dirty={fieldIsDirty("label")}
            placeholder={t("jira:label")}
            onChange={(e) => onPatch({ label: e.target.value })}
          />
        </div>
        <div className="flex flex-col gap-0.5 flex-1">
          <span className="text-[10px] text-muted-foreground">{t("jira:hint")}</span>
          <Input
            className="h-8"
            value={preset.hint}
            data-settings-dirty={fieldIsDirty("hint")}
            placeholder={t("jira:hintOptional")}
            onChange={(e) => onPatch({ hint: e.target.value })}
          />
        </div>
        <Button
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer text-xs"
          onClick={onToggle}
        >
          {expanded ? t("jira:hidePrompt") : t("jira:editPrompt")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer text-destructive"
          onClick={onRemove}
          aria-label={t("jira:remove")}
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
      {expanded && (
        <PresetPromptEditor
          preset={preset}
          isDirty={fieldIsDirty("prompt_template")}
          onPatch={onPatch}
        />
      )}
    </div>
  );
}

function PresetPromptEditor({
  preset,
  isDirty,
  onPatch,
}: {
  preset: JiraStoredPreset;
  isDirty: boolean;
  onPatch: (patch: Partial<JiraStoredPreset>) => void;
}) {
  const { t } = useTranslation();
  // Memoized because `ScriptEditor` keys its Monaco completion-provider
  // registration on `placeholders` identity. This used to be a module-scope
  // const, so it was stable for free; now that it is built from `t`, a fresh
  // array on every render would re-register the provider on every keystroke.
  const placeholders = useMemo(() => jiraPromptPlaceholders(t), [t]);
  return (
    <div className="px-2 pb-2 space-y-1">
      <div className="rounded-md border overflow-hidden" data-settings-dirty={isDirty}>
        <ScriptEditor
          value={preset.prompt_template}
          onChange={(v) => onPatch({ prompt_template: v })}
          language="markdown"
          height={computeEditorHeight(preset.prompt_template)}
          lineNumbers="off"
          placeholders={placeholders}
        />
      </div>
      <p className="text-[11px] text-muted-foreground/60">
        {/* The five `{{…}}` tokens are passed as values, never written into the
            catalog, where i18next would interpolate them away. */}
        <Trans
          i18nKey="jira:presetPromptPlaceholderHelp"
          values={{
            token: "{{",
            key: "{{key}}",
            url: "{{url}}",
            title: "{{title}}",
            description: "{{description}}",
          }}
        >
          Type {"{{token}}"} to see available placeholders.{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{key}}"}</code>,{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{url}}"}</code>,{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{title}}"}</code>, and{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{description}}"}</code> are
          substituted when the action runs.
        </Trans>
      </p>
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
  return { draft, baseline, setDraft, dirty, save, reset, discard, loaded };
}

export function TaskPresetsSection() {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { draft, baseline, setDraft, dirty, save, reset, discard, loaded } = usePresetDraft();
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
      toast({ description: t("jira:taskPresetsSaved"), variant: "success" });
    } catch {
      toast({ description: t("jira:failedToSaveTaskPresets"), variant: "error" });
      // Signals failure to the settings save coordinator, which only records the
      // contributor id — this message never reaches a user, so it is not copy.
      throw new Error("Failed to save task presets");
    }
  }, [save, t, toast]);

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
      title={t("jira:taskPresets")}
      // `/jira` is an in-app route, so it is interpolated rather than written
      // into the catalog where a locale could turn it into a dead link.
      description={t("jira:taskPresetsDescription", { route: MY_JIRA_ROUTE })}
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
            {t("common:reset")}
          </Button>
        </div>
      }
    >
      <SettingsCard isDirty={dirty} className="space-y-2 p-4" data-testid="jira-task-presets-card">
        {draft.map((preset, index) => (
          <PresetRow
            key={preset.id}
            preset={preset}
            savedPreset={baseline.find((saved) => saved.id === preset.id)}
            expanded={expandedId === preset.id}
            onToggle={() => setExpandedId((id) => (id === preset.id ? null : preset.id))}
            onPatch={(p) => patch(index, p)}
            onRemove={() => remove(index)}
          />
        ))}
        <Button size="sm" variant="outline" onClick={add} className="cursor-pointer">
          <IconPlus className="h-3.5 w-3.5 mr-1" />
          {t("jira:addPreset")}
        </Button>
      </SettingsCard>
    </SettingsSection>
  );
}
