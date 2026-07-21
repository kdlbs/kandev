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
              aria-label={`Action label ${index + 1}`}
              value={preset.label}
              placeholder="Label"
              onChange={(event) => patch(index, { label: event.target.value })}
            />
            <Input
              aria-label={`Action hint ${index + 1}`}
              value={preset.hint}
              placeholder="Short hint"
              onChange={(event) => patch(index, { hint: event.target.value })}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-11 w-11 cursor-pointer text-destructive sm:h-8 sm:w-8"
              aria-label={`Remove ${preset.label}`}
              onClick={() => onChange(presets.filter((_, current) => current !== index))}
            >
              <IconTrash className="h-4 w-4" />
            </Button>
          </div>
          <Textarea
            aria-label={`Action prompt ${index + 1}`}
            value={preset.prompt_template}
            placeholder="Prompt using {{url}} and {{title}}"
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
  const { presets, loading, update, reset } = useGitLabActionPresets(workspaceId);
  const drafts = usePresetDrafts(presets);
  const { toast } = useToast();
  const save = useCallback(async () => {
    try {
      const result = await update({ mr: drafts.mr, issue: drafts.issue });
      if (result) drafts.setBaseline({ mr: result.mr, issue: result.issue });
      toast({ description: "GitLab quick actions saved", variant: "success" });
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Failed to save quick actions",
        variant: "error",
      });
      throw error;
    }
  }, [drafts, toast, update]);
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
    invalidReason: valid ? undefined : "Every quick action needs a label and prompt.",
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
      toast({ description: "GitLab quick actions reset", variant: "success" });
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Failed to reset quick actions",
        variant: "error",
      });
    }
  };
  return (
    <SettingsSection
      title="Quick actions"
      description="Task prompts shown on the GitLab browse page for merge requests and issues."
      action={
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-11 cursor-pointer sm:h-8"
          disabled={loading}
          aria-label="Reset quick actions to defaults"
          onClick={() => void resetDefaults()}
        >
          <IconRefresh className="h-4 w-4" /> Reset
        </Button>
      }
    >
      <SettingsCard isDirty={drafts.dirty}>
        <CardContent className="pt-4">
          <Tabs defaultValue="mr">
            <TabsList className="w-full sm:w-auto">
              <TabsTrigger value="mr" className="flex-1 cursor-pointer sm:flex-none">
                Merge requests
              </TabsTrigger>
              <TabsTrigger value="issue" className="flex-1 cursor-pointer sm:flex-none">
                Issues
              </TabsTrigger>
            </TabsList>
            <TabsContent value="mr">
              <PresetList presets={drafts.mr} onChange={drafts.setMR} addLabel="Add MR action" />
            </TabsContent>
            <TabsContent value="issue">
              <PresetList
                presets={drafts.issue}
                onChange={drafts.setIssue}
                addLabel="Add issue action"
              />
            </TabsContent>
          </Tabs>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
