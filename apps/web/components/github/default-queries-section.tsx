"use client";

import { useState, useCallback, useMemo } from "react";
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
import {
  useDefaultQueryPresets,
  toStored,
  type StoredQueryPreset,
} from "@/components/github/my-github/use-default-query-presets";
import {
  PR_PRESETS as BUILTIN_PR_PRESETS,
  ISSUE_PRESETS as BUILTIN_ISSUE_PRESETS,
} from "@/components/github/my-github/search-bar";

function newPreset(): StoredQueryPreset {
  return {
    value: `q_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New query",
    filter: "",
    group: "inbox",
  };
}

function QueryRow({
  preset,
  baseline,
  onPatch,
  onRemove,
}: {
  preset: StoredQueryPreset;
  baseline?: StoredQueryPreset;
  onPatch: (patch: Partial<StoredQueryPreset>) => void;
  onRemove: () => void;
}) {
  return (
    <div
      className="flex items-center gap-2 rounded-md border p-2"
      data-settings-dirty={JSON.stringify(preset) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <div className="flex flex-col gap-0.5">
        <span className="text-[10px] text-muted-foreground">Label</span>
        <Input
          className="h-8 w-36"
          value={preset.label}
          data-settings-dirty={preset.label !== baseline?.label}
          placeholder="Label"
          onChange={(e) => onPatch({ label: e.target.value })}
        />
      </div>
      <div className="flex flex-col gap-0.5 flex-1">
        <span className="text-[10px] text-muted-foreground">Query</span>
        <Input
          className="h-8 font-mono text-xs"
          value={preset.filter}
          data-settings-dirty={preset.filter !== baseline?.filter}
          placeholder="e.g. review-requested:@me is:open"
          onChange={(e) => onPatch({ filter: e.target.value })}
        />
      </div>
      <div className="flex flex-col gap-0.5">
        <span className="text-[10px] text-muted-foreground">Group</span>
        <Select
          value={preset.group}
          onValueChange={(v) => onPatch({ group: v as "inbox" | "created" })}
        >
          <SelectTrigger
            className="h-8 w-28 cursor-pointer"
            data-settings-dirty={preset.group !== baseline?.group}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="inbox" className="cursor-pointer">
              Inbox
            </SelectItem>
            <SelectItem value="created" className="cursor-pointer">
              Created
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Button
        variant="ghost"
        size="icon"
        className="h-8 w-8 cursor-pointer text-destructive mt-3.5"
        onClick={onRemove}
        aria-label="Remove"
      >
        <IconTrash className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function QueryEditor({
  presets,
  baseline,
  onChange,
  addLabel,
}: {
  presets: StoredQueryPreset[];
  baseline: StoredQueryPreset[];
  onChange: (presets: StoredQueryPreset[]) => void;
  addLabel: string;
}) {
  const patch = useCallback(
    (index: number, change: Partial<StoredQueryPreset>) => {
      onChange(presets.map((p, i) => (i === index ? { ...p, ...change } : p)));
    },
    [presets, onChange],
  );
  const remove = useCallback(
    (index: number) => onChange(presets.filter((_, i) => i !== index)),
    [presets, onChange],
  );
  const add = useCallback(() => onChange([...presets, newPreset()]), [presets, onChange]);

  return (
    <div className="space-y-2">
      {presets.map((preset, index) => (
        <QueryRow
          key={preset.value}
          preset={preset}
          baseline={baseline.find((candidate) => candidate.value === preset.value)}
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

function useDefaultQueryDrafts(workspaceId?: string) {
  const { toast } = useToast();
  const { prPresets, issuePresets, save, reset, isCustomized, isReady } = useDefaultQueryPresets(
    workspaceId ?? null,
  );
  const [prDraft, setPrDraft] = useState<StoredQueryPreset[]>(prPresets);
  const [issueDraft, setIssueDraft] = useState<StoredQueryPreset[]>(issuePresets);
  const [prBaseline, setPrBaseline] = useState(prPresets);
  const [issueBaseline, setIssueBaseline] = useState(issuePresets);
  const [resetRequested, setResetRequested] = useState(false);

  const dirty = useMemo(
    () =>
      resetRequested ||
      JSON.stringify(prBaseline) !== JSON.stringify(prDraft) ||
      JSON.stringify(issueBaseline) !== JSON.stringify(issueDraft),
    [prBaseline, issueBaseline, prDraft, issueDraft, resetRequested],
  );

  const [syncedPr, setSyncedPr] = useState(prPresets);
  const [syncedIssue, setSyncedIssue] = useState(issuePresets);
  if (prPresets !== syncedPr && !dirty) {
    setSyncedPr(prPresets);
    setPrBaseline(prPresets);
    setPrDraft(prPresets);
    setResetRequested(false);
  }
  if (issuePresets !== syncedIssue && !dirty) {
    setSyncedIssue(issuePresets);
    setIssueBaseline(issuePresets);
    setIssueDraft(issuePresets);
    setResetRequested(false);
  }

  const handleSave = useCallback(async () => {
    const submittedPr = prDraft;
    const submittedIssue = issueDraft;
    try {
      if (resetRequested) await reset();
      else await save({ pr: submittedPr, issue: submittedIssue });
      setPrBaseline(submittedPr);
      setIssueBaseline(submittedIssue);
      setPrDraft((current) =>
        JSON.stringify(current) === JSON.stringify(submittedPr) ? submittedPr : current,
      );
      setIssueDraft((current) =>
        JSON.stringify(current) === JSON.stringify(submittedIssue) ? submittedIssue : current,
      );
      setResetRequested(false);
      toast({ description: "Default queries saved", variant: "success" });
    } catch {
      toast({ description: "Failed to save default queries", variant: "error" });
      throw new Error("Failed to save default queries");
    }
  }, [issueDraft, prDraft, reset, resetRequested, save, toast]);

  const handleReset = useCallback(() => {
    setPrDraft(toStored(BUILTIN_PR_PRESETS));
    setIssueDraft(toStored(BUILTIN_ISSUE_PRESETS));
    setResetRequested(true);
  }, []);
  const discard = useCallback(() => {
    setPrDraft(prBaseline);
    setIssueDraft(issueBaseline);
    setResetRequested(false);
  }, [prBaseline, issueBaseline]);
  const defaultPr = toStored(BUILTIN_PR_PRESETS);
  const defaultIssue = toStored(BUILTIN_ISSUE_PRESETS);
  const atDefaults =
    JSON.stringify(prDraft) === JSON.stringify(defaultPr) &&
    JSON.stringify(issueDraft) === JSON.stringify(defaultIssue);

  useSettingsSaveContributor({
    id: `github-default-queries:${workspaceId ?? "global"}`,
    revision: JSON.stringify([prDraft, issueDraft]),
    isDirty: dirty,
    canSave: isReady,
    invalidReason: isReady ? undefined : "Default queries are still loading.",
    save: handleSave,
    discard,
  });

  return {
    prDraft,
    issueDraft,
    prBaseline,
    issueBaseline,
    dirty,
    setPrDraft: (next: StoredQueryPreset[]) => {
      setResetRequested(false);
      setPrDraft(next);
    },
    setIssueDraft: (next: StoredQueryPreset[]) => {
      setResetRequested(false);
      setIssueDraft(next);
    },
    reset: handleReset,
    atDefaults,
    resetDisabled: atDefaults && !isCustomized,
  };
}

export function DefaultQueriesSection({ workspaceId }: { workspaceId?: string }) {
  const drafts = useDefaultQueryDrafts(workspaceId);

  return (
    <SettingsSection
      title="Default queries"
      description="Sidebar queries shown on /github for pull requests and issues."
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={drafts.reset}
            disabled={drafts.resetDisabled}
            className="cursor-pointer"
          >
            <IconRefresh className="h-3.5 w-3.5 mr-1" />
            Reset
          </Button>
        </div>
      }
    >
      <SettingsCard isDirty={drafts.dirty}>
        <CardContent className="pt-6">
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
              <QueryEditor
                presets={drafts.prDraft}
                baseline={drafts.prBaseline}
                onChange={drafts.setPrDraft}
                addLabel="Add PR query"
              />
            </TabsContent>
            <TabsContent value="issue">
              <QueryEditor
                presets={drafts.issueDraft}
                baseline={drafts.issueBaseline}
                onChange={drafts.setIssueDraft}
                addLabel="Add issue query"
              />
            </TabsContent>
          </Tabs>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
