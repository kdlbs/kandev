"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconPlus, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useToast } from "@/components/toast-provider";
import {
  getAzureDevOpsWorkspaceSettings,
  updateAzureDevOpsWorkspaceSettings,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsQueryPreset } from "@/lib/types/azure-devops";
import {
  DEFAULT_AZURE_PULL_REQUEST_QUERIES,
  DEFAULT_AZURE_WORK_ITEM_QUERIES,
} from "./azure-devops-workspace-defaults";

type QueryKind = "Work item" | "Pull request";

function newQuery(kind: QueryKind): AzureDevOpsQueryPreset {
  return {
    id: `query_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New query",
    group: "inbox",
    filters:
      kind === "Work item"
        ? {
            wiql: "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project",
            top: 50,
          }
        : { status: "active", creator: "", reviewer: "" },
  };
}

function stringFilter(preset: AzureDevOpsQueryPreset, key: string, fallback = ""): string {
  return typeof preset.filters[key] === "string" ? preset.filters[key] : fallback;
}

function numberFilter(preset: AzureDevOpsQueryPreset, key: string, fallback: number): number {
  return typeof preset.filters[key] === "number" ? preset.filters[key] : fallback;
}

function QueryGroupSelect({
  kind,
  index,
  value,
  dirty,
  onChange,
}: {
  kind: QueryKind;
  index: number;
  value: string;
  dirty: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <Select value={value === "created" ? "created" : "inbox"} onValueChange={onChange}>
      <SelectTrigger
        className="h-11 w-full cursor-pointer sm:h-8"
        aria-label={`${kind} query group ${index + 1}`}
        data-settings-dirty={dirty}
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
  );
}

function QueryField({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={`flex min-w-0 flex-col gap-0.5 ${className ?? ""}`}>
      <span className="text-[10px] text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

type QueryRowProps = {
  preset: AzureDevOpsQueryPreset;
  baseline?: AzureDevOpsQueryPreset;
  index: number;
  onPatch: (patch: Partial<AzureDevOpsQueryPreset>) => void;
  onPatchFilters: (patch: Record<string, unknown>) => void;
  onRemove: () => void;
};

function RemoveQueryButton({
  kind,
  index,
  onRemove,
}: {
  kind: QueryKind;
  index: number;
  onRemove: () => void;
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-11 w-full cursor-pointer text-destructive sm:h-8 sm:w-8"
      aria-label={`Remove ${kind.toLowerCase()} query ${index + 1}`}
      onClick={onRemove}
    >
      <IconTrash className="h-4 w-4" />
    </Button>
  );
}

function WorkItemQueryRow({
  preset,
  baseline,
  index,
  onPatch,
  onPatchFilters,
  onRemove,
}: QueryRowProps) {
  return (
    <div
      className="grid grid-cols-2 gap-2 rounded-md border p-3 sm:grid-cols-[10rem_minmax(0,1fr)_5rem_7rem_auto] sm:items-end sm:p-2"
      data-settings-dirty={JSON.stringify(preset) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <QueryField label="Label">
        <Input
          className="h-11 w-full sm:h-8"
          value={preset.label}
          aria-label={`Work item query label ${index + 1}`}
          data-settings-dirty={preset.label !== baseline?.label}
          onChange={(event) => onPatch({ label: event.target.value })}
        />
      </QueryField>
      <QueryField label="WIQL" className="col-span-2 row-start-2 sm:col-span-1 sm:row-auto">
        <Input
          className="h-11 w-full font-mono text-xs sm:h-8"
          value={stringFilter(preset, "wiql")}
          aria-label={`Work item query WIQL ${index + 1}`}
          data-settings-dirty={
            stringFilter(preset, "wiql") !== (baseline ? stringFilter(baseline, "wiql") : undefined)
          }
          onChange={(event) => onPatchFilters({ wiql: event.target.value })}
        />
      </QueryField>
      <QueryField label="Top">
        <Input
          type="number"
          min={1}
          max={200}
          className="h-11 w-full sm:h-8"
          value={numberFilter(preset, "top", 50)}
          aria-label={`Work item query top ${index + 1}`}
          onChange={(event) => onPatchFilters({ top: Number(event.target.value) || 1 })}
        />
      </QueryField>
      <QueryField label="Group">
        <QueryGroupSelect
          kind="Work item"
          index={index}
          value={preset.group}
          dirty={preset.group !== baseline?.group}
          onChange={(group) => onPatch({ group })}
        />
      </QueryField>
      <RemoveQueryButton kind="Work item" index={index} onRemove={onRemove} />
    </div>
  );
}

function PullRequestQueryRow({
  preset,
  baseline,
  index,
  onPatch,
  onPatchFilters,
  onRemove,
}: QueryRowProps) {
  return (
    <div
      className="grid grid-cols-2 gap-2 rounded-md border p-3 sm:grid-cols-[10rem_8rem_minmax(0,1fr)_minmax(0,1fr)_7rem_auto] sm:items-end sm:p-2"
      data-settings-dirty={JSON.stringify(preset) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <QueryField label="Label">
        <Input
          className="h-11 w-full sm:h-8"
          value={preset.label}
          aria-label={`Pull request query label ${index + 1}`}
          data-settings-dirty={preset.label !== baseline?.label}
          onChange={(event) => onPatch({ label: event.target.value })}
        />
      </QueryField>
      <QueryField label="Status">
        <Select
          value={stringFilter(preset, "status", "active")}
          onValueChange={(status) => onPatchFilters({ status })}
        >
          <SelectTrigger
            className="h-11 w-full cursor-pointer sm:h-8"
            aria-label={`Pull request query status ${index + 1}`}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="completed">Completed</SelectItem>
            <SelectItem value="abandoned">Abandoned</SelectItem>
            <SelectItem value="all">All</SelectItem>
          </SelectContent>
        </Select>
      </QueryField>
      <QueryField label="Creator" className="col-span-2 sm:col-span-1">
        <Input
          className="h-11 w-full sm:h-8"
          value={stringFilter(preset, "creator")}
          aria-label={`Pull request query creator ${index + 1}`}
          placeholder="@me or identity ID"
          onChange={(event) => onPatchFilters({ creator: event.target.value })}
        />
      </QueryField>
      <QueryField label="Reviewer" className="col-span-2 sm:col-span-1">
        <Input
          className="h-11 w-full sm:h-8"
          value={stringFilter(preset, "reviewer")}
          aria-label={`Pull request query reviewer ${index + 1}`}
          placeholder="@me or identity ID"
          onChange={(event) => onPatchFilters({ reviewer: event.target.value })}
        />
      </QueryField>
      <QueryField label="Group">
        <QueryGroupSelect
          kind="Pull request"
          index={index}
          value={preset.group}
          dirty={preset.group !== baseline?.group}
          onChange={(group) => onPatch({ group })}
        />
      </QueryField>
      <RemoveQueryButton kind="Pull request" index={index} onRemove={onRemove} />
    </div>
  );
}

function QueryEditor({
  kind,
  queries,
  baseline,
  onChange,
}: {
  kind: QueryKind;
  queries: AzureDevOpsQueryPreset[];
  baseline: AzureDevOpsQueryPreset[];
  onChange: (queries: AzureDevOpsQueryPreset[]) => void;
}) {
  const patch = (index: number, values: Partial<AzureDevOpsQueryPreset>) =>
    onChange(
      queries.map((query, current) => (current === index ? { ...query, ...values } : query)),
    );
  const patchFilters = (index: number, values: Record<string, unknown>) =>
    patch(index, { filters: { ...queries[index].filters, ...values } });
  const Row = kind === "Work item" ? WorkItemQueryRow : PullRequestQueryRow;
  return (
    <div className="space-y-2">
      {queries.map((query, index) => (
        <Row
          key={query.id}
          preset={query}
          baseline={baseline.find((candidate) => candidate.id === query.id)}
          index={index}
          onPatch={(values) => patch(index, values)}
          onPatchFilters={(values) => patchFilters(index, values)}
          onRemove={() => onChange(queries.filter((_, current) => current !== index))}
        />
      ))}
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-11 w-full cursor-pointer sm:h-8 sm:w-auto"
        onClick={() => onChange([...queries, newQuery(kind)])}
      >
        <IconPlus className="h-4 w-4" /> Add {kind === "Pull request" ? "PR" : "work item"} query
      </Button>
    </div>
  );
}

function useQueryDrafts(workspaceId: string) {
  const [workItems, setWorkItems] = useState(DEFAULT_AZURE_WORK_ITEM_QUERIES);
  const [pullRequests, setPullRequests] = useState(DEFAULT_AZURE_PULL_REQUEST_QUERIES);
  const [baseline, setBaseline] = useState({ workItems, pullRequests });
  const [loading, setLoading] = useState(true);
  const [resetRequested, setResetRequested] = useState(false);
  const dirty = useMemo(
    () =>
      resetRequested || JSON.stringify({ workItems, pullRequests }) !== JSON.stringify(baseline),
    [baseline, pullRequests, resetRequested, workItems],
  );

  useEffect(() => {
    let current = true;
    void getAzureDevOpsWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (!current) return;
        setWorkItems(settings.workItemQueries);
        setPullRequests(settings.pullRequestQueries);
        setBaseline({
          workItems: settings.workItemQueries,
          pullRequests: settings.pullRequestQueries,
        });
      })
      .catch(() => undefined)
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  const save = useCallback(async () => {
    const response = await updateAzureDevOpsWorkspaceSettings(
      workspaceId,
      resetRequested
        ? { pullRequestQueries: null, workItemQueries: null }
        : { pullRequestQueries: pullRequests, workItemQueries: workItems },
    );
    setWorkItems(response.workItemQueries);
    setPullRequests(response.pullRequestQueries);
    setBaseline({
      workItems: response.workItemQueries,
      pullRequests: response.pullRequestQueries,
    });
    setResetRequested(false);
  }, [pullRequests, resetRequested, workItems, workspaceId]);
  const reset = useCallback(() => {
    setWorkItems(DEFAULT_AZURE_WORK_ITEM_QUERIES);
    setPullRequests(DEFAULT_AZURE_PULL_REQUEST_QUERIES);
    setResetRequested(true);
  }, []);
  const discard = useCallback(() => {
    setWorkItems(baseline.workItems);
    setPullRequests(baseline.pullRequests);
    setResetRequested(false);
  }, [baseline]);

  return {
    workItems,
    pullRequests,
    setWorkItems,
    setPullRequests,
    baseline,
    loading,
    dirty,
    save,
    reset,
    discard,
  };
}

function validQueries(queries: AzureDevOpsQueryPreset[], kind: QueryKind): boolean {
  return (
    queries.length > 0 &&
    queries.every(
      (query) =>
        query.label.trim() &&
        (kind === "Pull request"
          ? stringFilter(query, "status", "active")
          : stringFilter(query, "wiql")),
    )
  );
}

export function AzureDevOpsDefaultQueriesSection({ workspaceId }: { workspaceId: string }) {
  const drafts = useQueryDrafts(workspaceId);
  const { toast } = useToast();
  const valid =
    validQueries(drafts.pullRequests, "Pull request") &&
    validQueries(drafts.workItems, "Work item");
  const save = useCallback(async () => {
    try {
      await drafts.save();
      toast({ description: "Azure DevOps default queries saved", variant: "success" });
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Failed to save default queries",
        variant: "error",
      });
      throw error;
    }
  }, [drafts, toast]);

  useSettingsSaveContributor({
    id: `azure-devops-default-queries:${workspaceId}`,
    revision: JSON.stringify([drafts.pullRequests, drafts.workItems]),
    isDirty: drafts.dirty,
    canSave: !drafts.loading && valid,
    invalidReason: valid ? undefined : "Every default query needs a label and valid filters.",
    save,
    discard: drafts.discard,
  });

  return (
    <SettingsSection
      title="Default queries"
      description="Query shortcuts shown on /azure-devops for pull requests and work items."
      action={
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-11 w-full cursor-pointer sm:h-8 sm:w-auto"
          disabled={drafts.loading}
          onClick={drafts.reset}
        >
          <IconRefresh className="h-4 w-4" /> Reset
        </Button>
      }
    >
      <SettingsCard isDirty={drafts.dirty} data-testid="azure-devops-default-queries-card">
        <CardContent className="pt-4 sm:pt-6">
          <fieldset disabled={drafts.loading} className="contents">
            <Tabs defaultValue="pull-request">
              <TabsList className="w-full sm:w-auto">
                <TabsTrigger value="pull-request" className="flex-1 cursor-pointer sm:flex-none">
                  Pull requests
                </TabsTrigger>
                <TabsTrigger value="work-item" className="flex-1 cursor-pointer sm:flex-none">
                  Work items
                </TabsTrigger>
              </TabsList>
              <TabsContent value="pull-request">
                <QueryEditor
                  kind="Pull request"
                  queries={drafts.pullRequests}
                  baseline={drafts.baseline.pullRequests}
                  onChange={drafts.setPullRequests}
                />
              </TabsContent>
              <TabsContent value="work-item">
                <QueryEditor
                  kind="Work item"
                  queries={drafts.workItems}
                  baseline={drafts.baseline.workItems}
                  onChange={drafts.setWorkItems}
                />
              </TabsContent>
            </Tabs>
          </fieldset>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
