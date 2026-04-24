"use client";

import { useState, useCallback, useMemo } from "react";
import { IconPlus, IconTrash, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@kandev/ui/tabs";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import {
  useDefaultQueryPresets,
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
  onPatch,
  onRemove,
}: {
  preset: StoredQueryPreset;
  onPatch: (patch: Partial<StoredQueryPreset>) => void;
  onRemove: () => void;
}) {
  return (
    <div className="flex items-center gap-2 rounded-md border p-2">
      <div className="flex flex-col gap-0.5">
        <span className="text-[10px] text-muted-foreground">Label</span>
        <Input
          className="h-8 w-36"
          value={preset.label}
          placeholder="Label"
          onChange={(e) => onPatch({ label: e.target.value })}
        />
      </div>
      <div className="flex flex-col gap-0.5 flex-1">
        <span className="text-[10px] text-muted-foreground">Query</span>
        <Input
          className="h-8 font-mono text-xs"
          value={preset.filter}
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
          <SelectTrigger className="h-8 w-28 cursor-pointer">
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
  onChange,
  addLabel,
}: {
  presets: StoredQueryPreset[];
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

function toStored(presets: typeof BUILTIN_PR_PRESETS): StoredQueryPreset[] {
  return presets.map(({ value, label, filter, group }) => ({ value, label, filter, group }));
}

export function DefaultQueriesSection() {
  const { toast } = useToast();
  const { prPresets, issuePresets, save, reset, isCustomized } = useDefaultQueryPresets();
  const [prDraft, setPrDraft] = useState<StoredQueryPreset[]>(prPresets);
  const [issueDraft, setIssueDraft] = useState<StoredQueryPreset[]>(issuePresets);

  // Sync drafts when external state changes
  const [syncedPr, setSyncedPr] = useState(prPresets);
  const [syncedIssue, setSyncedIssue] = useState(issuePresets);
  if (prPresets !== syncedPr) {
    setSyncedPr(prPresets);
    setPrDraft(prPresets);
  }
  if (issuePresets !== syncedIssue) {
    setSyncedIssue(issuePresets);
    setIssueDraft(issuePresets);
  }

  const dirty = useMemo(
    () =>
      JSON.stringify(prPresets) !== JSON.stringify(prDraft) ||
      JSON.stringify(issuePresets) !== JSON.stringify(issueDraft),
    [prPresets, issuePresets, prDraft, issueDraft],
  );

  const handleSave = useCallback(() => {
    save({ pr: prDraft, issue: issueDraft });
    toast({ description: "Default queries saved", variant: "success" });
  }, [save, prDraft, issueDraft, toast]);

  const handleReset = useCallback(() => {
    reset();
    setPrDraft(toStored(BUILTIN_PR_PRESETS));
    setIssueDraft(toStored(BUILTIN_ISSUE_PRESETS));
    toast({ description: "Default queries reset to defaults", variant: "success" });
  }, [reset, toast]);

  return (
    <SettingsSection
      title="Default queries"
      description="Sidebar queries shown on /github for pull requests and issues."
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={handleReset}
            disabled={!isCustomized}
            className="cursor-pointer"
          >
            <IconRefresh className="h-3.5 w-3.5 mr-1" />
            Reset
          </Button>
          <Button size="sm" onClick={handleSave} disabled={!dirty} className="cursor-pointer">
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
          <QueryEditor presets={prDraft} onChange={setPrDraft} addLabel="Add PR query" />
        </TabsContent>
        <TabsContent value="issue">
          <QueryEditor presets={issueDraft} onChange={setIssueDraft} addLabel="Add issue query" />
        </TabsContent>
      </Tabs>
    </SettingsSection>
  );
}
