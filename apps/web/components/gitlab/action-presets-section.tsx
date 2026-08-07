"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconPlus, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useToast } from "@/components/toast-provider";
import { useGitLabActionPresets } from "@/hooks/domains/gitlab/use-gitlab-action-presets";
import type { GitLabActionPreset, GitLabActionPresets } from "@/lib/types/gitlab";
import { useTranslation } from "react-i18next";

// `label` is PERSISTED to workspace settings as part of `GitLabActionPreset`, and
// the row below is immediately editable, so it must stay locale-neutral: a preset
// seeded in one locale and saved unedited would keep that locale's text forever.
// Same contract as `newPreset` in components/github/action-presets-section.tsx.
function newPreset(): GitLabActionPreset {
  return {
    id: `preset_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New action",
    hint: "",
    icon: "sparkle",
    prompt_template: "",
  };
}

function PresetList({
  presets,
  onChange,
  addLabel,
}: {
  presets: GitLabActionPreset[];
  onChange: (next: GitLabActionPreset[]) => void;
  addLabel: string;
}) {
  const { t } = useTranslation();
  const patch = (index: number, change: Partial<GitLabActionPreset>) =>
    onChange(
      presets.map((preset, current) => (current === index ? { ...preset, ...change } : preset)),
    );
  return (
    <div className="space-y-3">
      {presets.map((preset, index) => (
        <div key={preset.id} className="space-y-2 rounded-md border p-3">
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
            <Input
              aria-label={t("gitlab:actionLabel", { index: index + 1 })}
              value={preset.label}
              placeholder={t("gitlab:label")}
              onChange={(event) => patch(index, { label: event.target.value })}
            />
            <Input
              aria-label={t("gitlab:actionHint", { index: index + 1 })}
              value={preset.hint}
              placeholder={t("gitlab:shortHint")}
              onChange={(event) => patch(index, { hint: event.target.value })}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-11 w-11 cursor-pointer text-destructive sm:h-8 sm:w-8"
              aria-label={t("gitlab:remove", { label: preset.label })}
              onClick={() => onChange(presets.filter((_, current) => current !== index))}
            >
              <IconTrash className="h-4 w-4" />
            </Button>
          </div>
          <Textarea
            aria-label={t("gitlab:actionPrompt", { index: index + 1 })}
            value={preset.prompt_template}
            // The two `{{…}}` tokens are passed as values so they never reach the
            // catalog, where i18next would interpolate them away.
            placeholder={t("gitlab:promptUsingUrlAndTitle", {
              url: "{{url}}",
              title: "{{title}}",
            })}
            className="min-h-24 font-mono text-xs"
            onChange={(event) => patch(index, { prompt_template: event.target.value })}
          />
        </div>
      ))}
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-11 w-full cursor-pointer sm:h-8 sm:w-auto"
        onClick={() => onChange([...presets, newPreset()])}
      >
        <IconPlus className="h-4 w-4" />
        {addLabel}
      </Button>
    </div>
  );
}

function usePresetDrafts(presets: GitLabActionPresets | null | undefined) {
  const [mr, setMR] = useState<GitLabActionPreset[]>([]);
  const [issue, setIssue] = useState<GitLabActionPreset[]>([]);
  const [baseline, setBaseline] = useState({ mr, issue });
  const dirty = useMemo(
    () => JSON.stringify({ mr, issue }) !== JSON.stringify(baseline),
    [baseline, issue, mr],
  );
  useEffect(() => {
    if (!presets || dirty) return;
    setMR(presets.mr);
    setIssue(presets.issue);
    setBaseline({ mr: presets.mr, issue: presets.issue });
  }, [dirty, presets]);
  return { mr, issue, setMR, setIssue, baseline, setBaseline, dirty };
}

export function validActionPresets(presets: GitLabActionPreset[]): boolean {
  return presets.every(
    (preset) => Boolean(preset.label.trim()) && Boolean(preset.prompt_template.trim()),
  );
}

export function GitLabActionPresetsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { presets, loading, update, reset } = useGitLabActionPresets(workspaceId);
  const drafts = usePresetDrafts(presets);
  const { toast } = useToast();
  const save = useCallback(async () => {
    try {
      const result = await update({ mr: drafts.mr, issue: drafts.issue });
      if (result) drafts.setBaseline({ mr: result.mr, issue: result.issue });
      toast({ description: t("gitlab:gitlabQuickActionsSaved"), variant: "success" });
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : t("gitlab:failedToSaveQuickActions"),
        variant: "error",
      });
      throw error;
    }
  }, [drafts, t, toast, update]);
  const discard = useCallback(() => {
    drafts.setMR(drafts.baseline.mr);
    drafts.setIssue(drafts.baseline.issue);
  }, [drafts]);
  const valid = validActionPresets(drafts.mr) && validActionPresets(drafts.issue);
  useSettingsSaveContributor({
    id: `gitlab-action-presets:${workspaceId}`,
    revision: JSON.stringify([drafts.mr, drafts.issue]),
    isDirty: drafts.dirty,
    canSave: !loading && valid,
    invalidReason: valid ? undefined : t("gitlab:everyQuickActionNeedsALabel"),
    save,
    discard,
  });
  const resetDefaults = async () => {
    try {
      const result = await reset();
      if (result) {
        drafts.setMR(result.mr);
        drafts.setIssue(result.issue);
        drafts.setBaseline({ mr: result.mr, issue: result.issue });
      }
      toast({ description: t("gitlab:gitlabQuickActionsReset"), variant: "success" });
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : t("gitlab:failedToResetQuickActions"),
        variant: "error",
      });
    }
  };
  return (
    <SettingsSection
      title={t("gitlab:quickActions")}
      description={t("gitlab:taskPromptsShownOnTheGitlab")}
      action={
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-11 cursor-pointer sm:h-8"
          disabled={loading}
          aria-label={t("gitlab:resetQuickActionsToDefaults")}
          onClick={() => void resetDefaults()}
        >
          <IconRefresh className="h-4 w-4" /> {t("common:reset")}
        </Button>
      }
    >
      <SettingsCard isDirty={drafts.dirty}>
        <CardContent className="pt-4">
          <Tabs defaultValue="mr">
            <TabsList className="w-full sm:w-auto">
              <TabsTrigger value="mr" className="flex-1 cursor-pointer sm:flex-none">
                {t("gitlab:mergeRequests")}
              </TabsTrigger>
              <TabsTrigger value="issue" className="flex-1 cursor-pointer sm:flex-none">
                {t("gitlab:issues")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="mr">
              <PresetList
                presets={drafts.mr}
                onChange={drafts.setMR}
                addLabel={t("gitlab:addMrAction")}
              />
            </TabsContent>
            <TabsContent value="issue">
              <PresetList
                presets={drafts.issue}
                onChange={drafts.setIssue}
                addLabel={t("gitlab:addIssueAction")}
              />
            </TabsContent>
          </Tabs>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
