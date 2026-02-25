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
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
import {
  REVIEW_WATCH_PLACEHOLDERS,
  DEFAULT_REVIEW_WATCH_PROMPT,
} from "@/components/github/review-watch-placeholders";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import type {
  RepoFilter,
  ReviewWatch,
  CreateReviewWatchRequest,
  UpdateReviewWatchRequest,
} from "@/lib/types/github";

type ReviewWatchDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: ReviewWatch | null;
  workspaceId: string;
  onCreate: (req: CreateReviewWatchRequest) => Promise<void>;
  onUpdate: (id: string, req: UpdateReviewWatchRequest) => Promise<void>;
};

const QUERY_TEMPLATES = {
  meAndTeams: "type:pr state:open review-requested:@me -is:draft",
  me: "type:pr state:open user-review-requested:@me -is:draft",
} as const;

type FormState = {
  selectedRepos: RepoFilter[];
  allRepos: boolean;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  customQuery: string;
  enabled: boolean;
  pollInterval: number;
};

type WorkflowStepOption = { id: string; name: string };

const defaultFormState: FormState = {
  selectedRepos: [],
  allRepos: false,
  workflowId: "",
  workflowStepId: "",
  agentProfileId: "",
  executorProfileId: "",
  prompt: DEFAULT_REVIEW_WATCH_PROMPT,
  customQuery: QUERY_TEMPLATES.meAndTeams,
  enabled: true,
  pollInterval: 300,
};

function formStateFromWatch(watch: ReviewWatch): FormState {
  const hasRepos = watch.repos && watch.repos.length > 0;
  return {
    selectedRepos: hasRepos ? watch.repos : [],
    allRepos: !hasRepos,
    workflowId: watch.workflow_id,
    workflowStepId: watch.workflow_step_id,
    agentProfileId: watch.agent_profile_id,
    executorProfileId: watch.executor_profile_id,
    prompt: watch.prompt || DEFAULT_REVIEW_WATCH_PROMPT,
    customQuery: watch.custom_query || QUERY_TEMPLATES.meAndTeams,
    enabled: watch.enabled,
    pollInterval: watch.poll_interval_seconds,
  };
}

// --- Generic select field with description ---

type SelectFieldProps = {
  label: string;
  description?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: Array<{ id: string; label: string }>;
};

function SelectField({
  label,
  description,
  value,
  onChange,
  placeholder,
  items,
}: SelectFieldProps) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// --- Hooks ---

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
    () => executors.flatMap((e) => e.profiles ?? []),
    [executors],
  );

  // Filter out passthrough/TUI profiles — they don't accept initial prompts
  const filteredAgentProfiles = useMemo(
    () => agentProfiles.filter((p) => !p.cli_passthrough),
    [agentProfiles],
  );

  return { workflows, agentProfiles: filteredAgentProfiles, allExecutorProfiles };
}

// --- Section header ---

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

// --- Form field groups ---

function QueryField({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const current = form.customQuery.trim();
  const isMeAndTeams = current === QUERY_TEMPLATES.meAndTeams;
  const isMe = current === QUERY_TEMPLATES.me;

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label>Search Query</Label>
        <div className="flex items-center gap-1.5">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant={isMeAndTeams ? "default" : "secondary"}
                  size="sm"
                  className="h-6 px-2 text-xs cursor-pointer"
                  onClick={() =>
                    setForm((prev) => ({ ...prev, customQuery: QUERY_TEMPLATES.meAndTeams }))
                  }
                >
                  Me & my teams
                </Button>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs text-xs">
                PRs where you or any of your teams are requested as reviewers
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant={isMe ? "default" : "secondary"}
                  size="sm"
                  className="h-6 px-2 text-xs cursor-pointer"
                  onClick={() => setForm((prev) => ({ ...prev, customQuery: QUERY_TEMPLATES.me }))}
                >
                  Me
                </Button>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs text-xs">
                Only PRs where you are explicitly requested as a reviewer (not via team membership)
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>
      <Textarea
        value={form.customQuery}
        onChange={(e) => setForm((prev) => ({ ...prev, customQuery: e.target.value }))}
        placeholder="e.g. type:pr state:open review-requested:@me"
        rows={1}
        className="font-mono text-xs resize-y"
      />
      <p className="text-xs text-muted-foreground">
        GitHub search query. Supports full GitHub search syntax for maximum flexibility.
      </p>
    </div>
  );
}

function WatchFormFields({
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
  const { workflows, agentProfiles, allExecutorProfiles } = useWatchFormData(workspaceId);
  const workflowSteps = useWorkflowSteps(form.workflowId);

  const handleWorkflowChange = useCallback(
    (v: string) => {
      setForm((prev) => ({ ...prev, workflowId: v, workflowStepId: "" }));
    },
    [setForm],
  );

  return (
    <div className="space-y-5">
      <SectionHeader>Filter</SectionHeader>
      <RepoFilterSelector
        allRepos={form.allRepos}
        selectedRepos={form.selectedRepos}
        onAllReposChange={onAllReposChange}
        onSelectedReposChange={onSelectedReposChange}
      />
      <QueryField form={form} setForm={setForm} />
      <SectionHeader>Automation</SectionHeader>
      <WorkflowFields
        form={form}
        setForm={setForm}
        workflows={workflows}
        workflowSteps={workflowSteps}
        onWorkflowChange={handleWorkflowChange}
      />
      <ProfileFields
        form={form}
        setForm={setForm}
        agentProfiles={agentProfiles}
        executorProfiles={allExecutorProfiles}
      />
      <PromptField form={form} setForm={setForm} />
      <SectionHeader>Settings</SectionHeader>
      <SettingsFields form={form} setForm={setForm} />
    </div>
  );
}

function WorkflowFields({
  form,
  setForm,
  workflows,
  workflowSteps,
  onWorkflowChange,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  workflows: Array<{ id: string; name: string }>;
  workflowSteps: WorkflowStepOption[];
  onWorkflowChange: (v: string) => void;
}) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label="Workflow"
        description="The workflow to create tasks in."
        value={form.workflowId}
        onChange={onWorkflowChange}
        placeholder="Select workflow"
        items={workflows.map((w) => ({ id: w.id, label: w.name }))}
      />
      <SelectField
        label="Workflow Step"
        description="Initial step for new tasks. Auto-start is set on the step."
        value={form.workflowStepId}
        onChange={(v) => setForm((prev) => ({ ...prev, workflowStepId: v }))}
        placeholder={form.workflowId ? "Select step" : "Select a workflow first"}
        items={workflowSteps.map((s) => ({ id: s.id, label: s.name }))}
      />
    </div>
  );
}

function HelpTip({ text }: { text: string }) {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs text-xs">{text}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function ProfileFields({
  form,
  setForm,
  agentProfiles,
  executorProfiles,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  agentProfiles: Array<{ id: string; label: string }>;
  executorProfiles: Array<{ id: string; name: string }>;
}) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label="Agent Profile"
        description="The agent configuration used to review the PR."
        value={form.agentProfileId}
        onChange={(v) => setForm((prev) => ({ ...prev, agentProfileId: v }))}
        placeholder="Select agent profile"
        items={agentProfiles.map((p) => ({ id: p.id, label: p.label }))}
      />
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          <Label>Executor Profile</Label>
          <HelpTip text="The repository will be automatically cloned to ~/.kandev/repos/<owner>/<repo> if it is not already present in the workspace." />
        </div>
        <p className="text-xs text-muted-foreground">
          The executor environment where the agent will run.
        </p>
        <Select
          value={form.executorProfileId || undefined}
          onValueChange={(v) => setForm((prev) => ({ ...prev, executorProfileId: v }))}
        >
          <SelectTrigger className="cursor-pointer">
            <SelectValue placeholder="Select executor profile" />
          </SelectTrigger>
          <SelectContent>
            {executorProfiles.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
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
            {REVIEW_WATCH_PLACEHOLDERS.map((p) => (
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

function PromptField({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>Task Prompt</Label>
        <PlaceholdersHelp />
      </div>
      <p className="text-xs text-muted-foreground">
        The prompt sent to the agent for each new PR. Type {"{{"} to insert placeholders.
      </p>
      <div className="rounded-md border border-border overflow-hidden">
        <ScriptEditor
          value={form.prompt}
          onChange={(v) => setForm((prev) => ({ ...prev, prompt: v }))}
          language="plaintext"
          height="120px"
          lineNumbers="off"
          placeholders={REVIEW_WATCH_PLACEHOLDERS}
        />
      </div>
      <p className="text-xs text-muted-foreground/70">
        The workflow step prompt wraps this prompt. For example, if the step prompt is{" "}
        <code className="text-[10px] bg-muted px-1 rounded">
          {"Analyze the task: {{task_prompt}}"}
        </code>
        , the final prompt becomes{" "}
        <code className="text-[10px] bg-muted px-1 rounded">
          {"Analyze the task: Pull Request ready for review: https://..."}
        </code>
      </p>
    </div>
  );
}

function SettingsFields({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  return (
    <>
      <div className="space-y-1.5">
        <Label>Poll Interval (seconds)</Label>
        <p className="text-xs text-muted-foreground">
          How often to check for new PRs. Minimum 60s, maximum 3600s.
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

function getSaveButtonLabel(saving: boolean, isEditing: boolean): string {
  if (saving) return "Saving...";
  return isEditing ? "Update" : "Create";
}

export function ReviewWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: ReviewWatchDialogProps) {
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

  const canSave =
    form.customQuery.trim().length > 0 &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    form.prompt.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[900px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{watch ? "Edit Review Watch" : "Create Review Watch"}</DialogTitle>
          <DialogDescription>
            Automatically create tasks when new pull requests need your review.
          </DialogDescription>
        </DialogHeader>
        <WatchFormFields
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
            {getSaveButtonLabel(saving, !!watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
