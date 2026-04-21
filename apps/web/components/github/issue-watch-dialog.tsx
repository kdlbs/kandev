"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
  ISSUE_WATCH_PLACEHOLDERS,
  DEFAULT_ISSUE_WATCH_PROMPT,
} from "@/components/github/issue-watch-placeholders";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import type {
  RepoFilter,
  IssueWatch,
  CreateIssueWatchRequest,
  UpdateIssueWatchRequest,
} from "@/lib/types/github";

type IssueWatchDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: IssueWatch | null;
  workspaceId: string;
  onCreate: (req: CreateIssueWatchRequest) => Promise<void>;
  onUpdate: (id: string, req: UpdateIssueWatchRequest) => Promise<void>;
};

type FormState = {
  selectedRepos: RepoFilter[];
  allRepos: boolean;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  labels: string;
  customQuery: string;
  enabled: boolean;
  pollInterval: number;
};

type WorkflowStepOption = { id: string; name: string };

const DEFAULT_QUERY = "type:issue state:open";

const defaultFormState: FormState = {
  selectedRepos: [],
  allRepos: false,
  workflowId: "",
  workflowStepId: "",
  agentProfileId: "",
  executorProfileId: "",
  prompt: DEFAULT_ISSUE_WATCH_PROMPT,
  labels: "",
  customQuery: DEFAULT_QUERY,
  enabled: true,
  pollInterval: 300,
};

function formStateFromWatch(watch: IssueWatch): FormState {
  const hasRepos = watch.repos && watch.repos.length > 0;
  return {
    selectedRepos: hasRepos ? watch.repos : [],
    allRepos: !hasRepos,
    workflowId: watch.workflow_id,
    workflowStepId: watch.workflow_step_id,
    agentProfileId: watch.agent_profile_id,
    executorProfileId: watch.executor_profile_id,
    prompt: watch.prompt || DEFAULT_ISSUE_WATCH_PROMPT,
    labels: (watch.labels ?? []).join(", "),
    customQuery: watch.custom_query || DEFAULT_QUERY,
    enabled: watch.enabled,
    pollInterval: watch.poll_interval_seconds,
  };
}

function useWorkflowSteps(workflowId: string) {
  const [steps, setSteps] = useState<WorkflowStepOption[]>([]);

  useEffect(() => {
    if (!workflowId) {
      void Promise.resolve().then(() => setSteps([]));
      return;
    }
    let cancelled = false;
    listWorkflowSteps(workflowId)
      .then((response) => {
        if (cancelled) return;
        const sorted = [...response.steps].sort((a, b) => a.position - b.position);
        setSteps(sorted.map((s) => ({ id: s.id, name: s.name })));
      })
      .catch(() => {
        if (!cancelled) setSteps([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workflowId]);

  return steps;
}

function useWatchFormData(workspaceId: string) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);

  const workflows = useAppStore((state) => state.workflows.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const allExecutorProfiles = useMemo(
    () => executors.filter((e) => e.type !== "local").flatMap((e) => e.profiles ?? []),
    [executors],
  );

  const filteredAgentProfiles = useMemo(
    () => agentProfiles.filter((p) => !p.cli_passthrough),
    [agentProfiles],
  );

  return { workflows, agentProfiles: filteredAgentProfiles, allExecutorProfiles };
}

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 pt-1">
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider shrink-0">
        {children}
      </span>
      <div className="flex-1 border-t border-border" />
    </div>
  );
}

function PlaceholdersHelp() {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">Available placeholders:</p>
          <ul className="text-xs space-y-0.5">
            {ISSUE_WATCH_PLACEHOLDERS.map((p) => (
              <li key={p.key}>
                <code className="text-[10px] bg-white/15 px-1 rounded">{`{{${p.key}}}`}</code>{" "}
                <span className="opacity-70">{p.description}</span>
              </li>
            ))}
          </ul>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function IssueWatchFormFields({
  form,
  setForm,
  workspaceId,
  onAllReposChange,
  onSelectedReposChange,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  workspaceId: string;
  onAllReposChange: (checked: boolean) => void;
  onSelectedReposChange: (repos: RepoFilter[]) => void;
}) {
  return (
    <div className="space-y-5">
      <IssueFilterFields
        form={form}
        setForm={setForm}
        onAllReposChange={onAllReposChange}
        onSelectedReposChange={onSelectedReposChange}
      />
      <IssueAutomationFields form={form} setForm={setForm} workspaceId={workspaceId} />
      <IssueSettingsFields form={form} setForm={setForm} />
    </div>
  );
}

function IssueFilterFields({
  form,
  setForm,
  onAllReposChange,
  onSelectedReposChange,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  onAllReposChange: (checked: boolean) => void;
  onSelectedReposChange: (repos: RepoFilter[]) => void;
}) {
  return (
    <>
      <SectionHeader>Filter</SectionHeader>
      <RepoFilterSelector
        allRepos={form.allRepos}
        selectedRepos={form.selectedRepos}
        onAllReposChange={onAllReposChange}
        onSelectedReposChange={onSelectedReposChange}
      />
      <div className="space-y-1.5">
        <Label>Labels (comma-separated)</Label>
        <Input
          value={form.labels}
          onChange={(e) => setForm((prev) => ({ ...prev, labels: e.target.value }))}
          placeholder="e.g. bug, enhancement, priority:high"
        />
        <p className="text-xs text-muted-foreground">
          Only match issues with these labels. Leave empty for all issues.
        </p>
      </div>
      <div className="space-y-1.5">
        <Label>Custom Query</Label>
        <Textarea
          value={form.customQuery}
          onChange={(e) => setForm((prev) => ({ ...prev, customQuery: e.target.value }))}
          placeholder="e.g. type:issue state:open label:bug"
          rows={1}
          className="font-mono text-xs resize-y"
        />
        <p className="text-xs text-muted-foreground">
          GitHub search query. When set, overrides the label filter above.
        </p>
      </div>
    </>
  );
}

function IssueAutomationFields({
  form,
  setForm,
  workspaceId,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  workspaceId: string;
}) {
  const { workflows, agentProfiles, allExecutorProfiles } = useWatchFormData(workspaceId);
  const workflowSteps = useWorkflowSteps(form.workflowId);

  return (
    <>
      <SectionHeader>Automation</SectionHeader>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label="Workflow"
          description="The workflow to create tasks in."
          value={form.workflowId}
          onChange={(v) => setForm((prev) => ({ ...prev, workflowId: v, workflowStepId: "" }))}
          placeholder="Select workflow"
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label="Workflow Step"
          description="Initial step for new tasks."
          value={form.workflowStepId}
          onChange={(v) => setForm((prev) => ({ ...prev, workflowStepId: v }))}
          placeholder={form.workflowId ? "Select step" : "Select a workflow first"}
          items={workflowSteps.map((s) => ({ id: s.id, label: s.name }))}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label="Agent Profile"
          description="The agent configuration for the task."
          value={form.agentProfileId}
          onChange={(v) => setForm((prev) => ({ ...prev, agentProfileId: v }))}
          placeholder="Select agent profile"
          items={agentProfiles.map((p) => ({ id: p.id, label: p.label }))}
        />
        <SelectField
          label="Executor Profile"
          description="The executor environment for the agent."
          value={form.executorProfileId}
          onChange={(v) => setForm((prev) => ({ ...prev, executorProfileId: v }))}
          placeholder="Select executor profile"
          items={allExecutorProfiles.map((p) => ({ id: p.id, label: p.name }))}
        />
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          <Label>Task Prompt</Label>
          <PlaceholdersHelp />
        </div>
        <p className="text-xs text-muted-foreground">
          The prompt sent to the agent for each new issue. Type {"{{"} to insert placeholders.
        </p>
        <div className="rounded-md border border-border overflow-hidden">
          <ScriptEditor
            value={form.prompt}
            onChange={(v) => setForm((prev) => ({ ...prev, prompt: v }))}
            language="markdown"
            height={computeEditorHeight(form.prompt)}
            lineNumbers="off"
            placeholders={ISSUE_WATCH_PLACEHOLDERS}
          />
        </div>
      </div>
    </>
  );
}

function IssueSettingsFields({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  return (
    <>
      <SectionHeader>Settings</SectionHeader>
      <div className="space-y-1.5">
        <Label>Poll Interval (seconds)</Label>
        <p className="text-xs text-muted-foreground">
          How often to check for new issues. Minimum 60s, maximum 3600s.
        </p>
        <Input
          type="number"
          value={form.pollInterval}
          onChange={(e) => setForm((prev) => ({ ...prev, pollInterval: Number(e.target.value) }))}
          min={60}
          max={3600}
        />
      </div>
      <div className="flex items-center justify-between">
        <div>
          <Label>Enabled</Label>
          <p className="text-xs text-muted-foreground">Pause or resume polling.</p>
        </div>
        <Switch
          checked={form.enabled}
          onCheckedChange={(v) => setForm((prev) => ({ ...prev, enabled: v }))}
          className="cursor-pointer"
        />
      </div>
    </>
  );
}

function parseLabels(labelsStr: string): string[] {
  return labelsStr
    .split(",")
    .map((l) => l.trim())
    .filter((l) => l.length > 0);
}

function getSaveLabel(watch: IssueWatch | null | undefined): string {
  return watch ? "Update" : "Create";
}

export function IssueWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: IssueWatchDialogProps) {
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(defaultFormState);

  useEffect(() => {
    setForm(watch ? formStateFromWatch(watch) : defaultFormState);
  }, [watch, open]);

  const handleSelectedReposChange = useCallback((repos: RepoFilter[]) => {
    setForm((prev) => ({ ...prev, selectedRepos: repos }));
  }, []);

  const handleAllReposChange = useCallback((checked: boolean) => {
    setForm((prev) => ({
      ...prev,
      allRepos: checked,
      selectedRepos: checked ? [] : prev.selectedRepos,
    }));
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const repos = form.allRepos ? [] : form.selectedRepos;
      const payload = {
        workflow_id: form.workflowId,
        workflow_step_id: form.workflowStepId,
        repos,
        agent_profile_id: form.agentProfileId,
        executor_profile_id: form.executorProfileId,
        prompt: form.prompt,
        labels: parseLabels(form.labels),
        custom_query: form.customQuery,
        enabled: form.enabled,
        poll_interval_seconds: form.pollInterval,
      };
      if (watch) {
        await onUpdate(watch.id, payload);
      } else {
        await onCreate({ ...payload, workspace_id: workspaceId });
      }
      onOpenChange(false);
    } catch {
      // Error handled by caller
    } finally {
      setSaving(false);
    }
  }, [form, watch, workspaceId, onCreate, onUpdate, onOpenChange]);

  const canSave = !!form.workflowId && !!form.workflowStepId && form.prompt.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[900px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{watch ? "Edit Issue Watch" : "Create Issue Watch"}</DialogTitle>
          <DialogDescription>
            Automatically create tasks when new GitHub issues match your criteria.
          </DialogDescription>
        </DialogHeader>
        <IssueWatchFormFields
          form={form}
          setForm={setForm}
          workspaceId={workspaceId}
          onAllReposChange={handleAllReposChange}
          onSelectedReposChange={handleSelectedReposChange}
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving || !canSave} className="cursor-pointer">
            {saving ? "Saving..." : getSaveLabel(watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SelectField(props: {
  label: string;
  description?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: { id: string; label: string }[];
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      {props.description && <p className="text-xs text-muted-foreground">{props.description}</p>}
      <Select value={props.value || undefined} onValueChange={props.onChange}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={props.placeholder} />
        </SelectTrigger>
        <SelectContent>
          {props.items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
