"use client";

import { createElement, useMemo, useState, useCallback } from "react";
import { IconPlus, IconTrash, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@kandev/ui/tabs";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
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
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

// A function, not a const: `description` is copy, and a module-scope `t()` call
// would freeze at the boot locale (see docs/i18n.md). `key` is the substitution
// token and `example` is sample GitHub data — neither is translated.
function actionPromptPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "url",
      description: t("github:placeholderActionUrl"),
      example: "https://github.com/org/repo/pull/42",
      executor_types: [],
    },
    {
      key: "title",
      description: t("github:placeholderActionTitle"),
      example: "Fix login page crash",
      executor_types: [],
    },
  ];
}

// `label` is PERSISTED to workspace settings as part of `GitHubActionPreset` and
// is immediately editable in the row below, so it must stay locale-neutral: a
// preset seeded in one locale and saved unedited would keep that locale's text
// forever. Same contract as DEFAULT_PR_PRESETS in my-github/action-presets.ts.
function newPreset(): GitHubActionPreset {
  return {
    id: `preset_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New action",
    hint: "",
    icon: "sparkle",
    prompt_template: "",
  };
}

function PresetIconSelect({
  value,
  isDirty,
  onChange,
}: {
  value: string;
  isDirty: boolean;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className="!h-8 py-0.5 text-sm cursor-pointer"
        aria-label={t("github:icon")}
        data-settings-dirty={isDirty}
      >
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
  baseline,
  expanded,
  onToggle,
  onPatch,
  onRemove,
}: {
  preset: GitHubActionPreset;
  baseline?: GitHubActionPreset;
  expanded: boolean;
  onToggle: () => void;
  onPatch: (patch: Partial<GitHubActionPreset>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="rounded-md border"
      data-settings-dirty={JSON.stringify(preset) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <div className="flex items-end gap-2 p-2">
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">{t("github:icon")}</span>
          <PresetIconSelect
            value={preset.icon}
            isDirty={preset.icon !== baseline?.icon}
            onChange={(v) => onPatch({ icon: v })}
          />
        </div>
        <div className="flex flex-col gap-0.5">
          <span className="text-[10px] text-muted-foreground">{t("github:label")}</span>
          <Input
            className="h-8 w-40"
            value={preset.label}
            data-settings-dirty={preset.label !== baseline?.label}
            placeholder={t("github:label")}
            onChange={(e) => onPatch({ label: e.target.value })}
          />
        </div>
        <div className="flex flex-col gap-0.5 flex-1">
          <span className="text-[10px] text-muted-foreground">{t("github:hint")}</span>
          <Input
            className="h-8"
            value={preset.hint}
            data-settings-dirty={preset.hint !== baseline?.hint}
            placeholder={t("github:hintOptional")}
            onChange={(e) => onPatch({ hint: e.target.value })}
          />
        </div>
        <Button
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer text-xs"
          onClick={onToggle}
        >
          {expanded ? t("github:hidePrompt") : t("github:editPrompt")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer text-destructive"
          onClick={onRemove}
          aria-label={t("github:remove")}
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
      {expanded && <PresetPromptEditor preset={preset} baseline={baseline} onPatch={onPatch} />}
    </div>
  );
}

// Split out of PresetRow to keep that component under the 100-line lint cap.
function PresetPromptEditor({
  preset,
  baseline,
  onPatch,
}: {
  preset: GitHubActionPreset;
  baseline?: GitHubActionPreset;
  onPatch: (patch: Partial<GitHubActionPreset>) => void;
}) {
  const { t } = useTranslation();
  const placeholders = useMemo(() => actionPromptPlaceholders(t), [t]);
  return (
    <div className="px-2 pb-2 space-y-1">
      <div
        className="rounded-md border overflow-hidden"
        data-settings-dirty={preset.prompt_template !== baseline?.prompt_template}
        data-settings-dirty-level="container"
      >
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
        {/* The three `{{…}}` tokens are passed as values, never written into the
            catalog, where i18next would interpolate them away. */}
        <Trans
          i18nKey="github:actionPromptPlaceholderHelp"
          values={{ token: "{{", url: "{{url}}", title: "{{title}}" }}
        >
          Type {"{{token}}"} to see available placeholders.{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{url}}"}</code> and{" "}
          <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{title}}"}</code> are
          substituted when the action runs.
        </Trans>
      </p>
    </div>
  );
}

function PresetEditor({
  presets,
  baseline,
  onChange,
  addLabel,
}: {
  presets: GitHubActionPreset[];
  baseline: GitHubActionPreset[];
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
          baseline={baseline.find((candidate) => candidate.id === preset.id)}
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
  prBaseline: GitHubActionPreset[];
  issueBaseline: GitHubActionPreset[];
  save: () => Promise<void>;
  reset: () => void;
  discard: () => void;
  loading: boolean;
} {
  const { presets, save, loading } = useGitHubActionPresets(workspaceId);
  const [prDraft, setPrDraft] = useState<GitHubActionPreset[]>(() =>
    presets?.pr?.length ? presets.pr : DEFAULT_PR_PRESETS,
  );
  const [issueDraft, setIssueDraft] = useState<GitHubActionPreset[]>(() =>
    presets?.issue?.length ? presets.issue : DEFAULT_ISSUE_PRESETS,
  );
  const [prBaseline, setPrBaseline] = useState(prDraft);
  const [issueBaseline, setIssueBaseline] = useState(issueDraft);
  const dirty = useMemo(
    () =>
      JSON.stringify(prBaseline) !== JSON.stringify(prDraft) ||
      JSON.stringify(issueBaseline) !== JSON.stringify(issueDraft),
    [prBaseline, issueBaseline, prDraft, issueDraft],
  );
  const [syncedPresets, setSyncedPresets] = useState(presets);
  if (presets && presets !== syncedPresets && !dirty) {
    const nextPr = presets.pr?.length ? presets.pr : DEFAULT_PR_PRESETS;
    const nextIssue = presets.issue?.length ? presets.issue : DEFAULT_ISSUE_PRESETS;
    setSyncedPresets(presets);
    setPrBaseline(nextPr);
    setIssueBaseline(nextIssue);
    setPrDraft(nextPr);
    setIssueDraft(nextIssue);
  }

  const persist = useCallback(async () => {
    const submittedPr = prDraft;
    const submittedIssue = issueDraft;
    const response = await save({ pr: submittedPr, issue: submittedIssue });
    const savedPr = response?.pr?.length ? response.pr : submittedPr;
    const savedIssue = response?.issue?.length ? response.issue : submittedIssue;
    setPrBaseline(savedPr);
    setIssueBaseline(savedIssue);
    setPrDraft((current) =>
      JSON.stringify(current) === JSON.stringify(submittedPr) ? savedPr : current,
    );
    setIssueDraft((current) =>
      JSON.stringify(current) === JSON.stringify(submittedIssue) ? savedIssue : current,
    );
  }, [save, prDraft, issueDraft]);

  const doReset = useCallback(() => {
    setPrDraft(DEFAULT_PR_PRESETS);
    setIssueDraft(DEFAULT_ISSUE_PRESETS);
  }, []);
  const discard = useCallback(() => {
    setPrDraft(prBaseline);
    setIssueDraft(issueBaseline);
  }, [prBaseline, issueBaseline]);

  return {
    prDraft,
    issueDraft,
    setPrDraft,
    setIssueDraft,
    dirty,
    prBaseline,
    issueBaseline,
    save: persist,
    reset: doReset,
    discard,
    loading,
  };
}

export function ActionPresetsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const {
    prDraft,
    issueDraft,
    prBaseline,
    issueBaseline,
    setPrDraft,
    setIssueDraft,
    dirty,
    save,
    reset,
    discard,
    loading,
  } = usePresetDrafts(workspaceId);
  const handleSave = useCallback(async () => {
    try {
      await save();
      toast({ description: t("github:quickActionsSaved"), variant: "success" });
    } catch {
      toast({ description: t("github:failedToSaveQuickActions"), variant: "error" });
      throw new Error("Failed to save quick actions");
    }
  }, [save, t, toast]);
  useSettingsSaveContributor({
    id: `github-action-presets:${workspaceId}`,
    revision: JSON.stringify([prDraft, issueDraft]),
    isDirty: dirty,
    canSave: !loading,
    invalidReason: loading ? t("github:quickActionsAreStillLoading") : undefined,
    save: handleSave,
    discard,
  });

  return (
    <SettingsSection
      title={t("github:quickActions")}
      description={t("github:promptsShownOnGithubWhenStarting", { route: "/github" })}
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={reset}
            disabled={loading}
            className="cursor-pointer"
          >
            <IconRefresh className="h-3.5 w-3.5 mr-1" />
            {t("common:reset")}
          </Button>
        </div>
      }
    >
      <SettingsCard isDirty={dirty}>
        <CardContent className="pt-6">
          <fieldset disabled={loading} className="contents">
            <Tabs defaultValue="pr">
              <TabsList>
                <TabsTrigger value="pr" className="cursor-pointer">
                  {t("github:pullRequests")}
                </TabsTrigger>
                <TabsTrigger value="issue" className="cursor-pointer">
                  {t("github:issues")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="pr">
                <PresetEditor
                  presets={prDraft}
                  baseline={prBaseline}
                  onChange={setPrDraft}
                  addLabel={t("github:addPrAction")}
                />
              </TabsContent>
              <TabsContent value="issue">
                <PresetEditor
                  presets={issueDraft}
                  baseline={issueBaseline}
                  onChange={setIssueDraft}
                  addLabel={t("github:addIssueAction")}
                />
              </TabsContent>
            </Tabs>
          </fieldset>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
