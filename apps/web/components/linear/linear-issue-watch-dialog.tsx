"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import { listLinearTeams, listLinearStates } from "@/lib/api/domains/linear-api";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
  LINEAR_ISSUE_WATCH_PLACEHOLDERS,
  DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
} from "./linear-issue-watch-placeholders";
import type {
  CreateLinearIssueWatchInput,
  LinearIssueWatch,
  LinearTeam,
  LinearWorkflowState,
  UpdateLinearIssueWatchInput,
} from "@/lib/types/linear";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: LinearIssueWatch | null;
  workspaceId?: string;
  onCreate: (req: CreateLinearIssueWatchInput) => Promise<unknown>;
  onUpdate: (id: string, req: UpdateLinearIssueWatchInput) => Promise<unknown>;
};

type FormState = {
  workspaceId: string;
  query: string;
  teamKey: string;
  stateIds: string[];
  assigned: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollInterval: number;
};

const ASSIGNED_ANY = "__any__";

function makeEmptyForm(workspaceId: string): FormState {
  return {
    workspaceId,
    query: "",
    teamKey: "",
    stateIds: [],
    assigned: "",
    workflowId: "",
    workflowStepId: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
    enabled: true,
    pollInterval: 300,
  };
}

function formStateFromWatch(w: LinearIssueWatch): FormState {
  return {
    workspaceId: w.workspaceId,
    query: w.filter?.query ?? "",
    teamKey: w.filter?.teamKey ?? "",
    stateIds: w.filter?.stateIds ?? [],
    assigned: w.filter?.assigned ?? "",
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
    agentProfileId: w.agentProfileId,
    executorProfileId: w.executorProfileId,
    prompt: w.prompt || DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
    enabled: w.enabled,
    pollInterval: w.pollIntervalSeconds,
  };
}

function useFormData(workspaceId: string) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);
  const allWorkflows = useAppStore((s) => s.workflows.items);
  const workflows = useMemo(() => allWorkflows.filter((w) => !w.hidden), [allWorkflows]);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const executors = useAppStore((s) => s.executors.items);
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

function SelectField(props: {
  label: string;
  description?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: { id: string; label: string }[];
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      {props.description && <p className="text-xs text-muted-foreground">{props.description}</p>}
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
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

function StateMultiSelect({
  states,
  loading,
  selected,
  onToggle,
  disabled,
}: {
  states: LinearWorkflowState[];
  loading: boolean;
  selected: string[];
  onToggle: (id: string) => void;
  disabled: boolean;
}) {
  if (disabled) {
    return (
      <p className="text-xs text-muted-foreground">
        Pick a team to choose specific workflow states.
      </p>
    );
  }
  if (loading) {
    return <p className="text-xs text-muted-foreground">Loading states…</p>;
  }
  if (states.length === 0) {
    return <p className="text-xs text-muted-foreground">No workflow states available.</p>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {states.map((s) => {
        const active = selected.includes(s.id);
        return (
          <Badge
            key={s.id}
            variant={active ? "default" : "outline"}
            className="cursor-pointer"
            onClick={() => onToggle(s.id)}
          >
            {s.name}
          </Badge>
        );
      })}
    </div>
  );
}

// useTeamsAndStates loads the team list once Linear is configured, plus the
// states for the currently-selected team. The states map is keyed by teamKey
// so switching teams renders an empty list without us having to setState in
// an effect — only the lookup expression changes.
function useTeamsAndStates(teamKey: string) {
  const [teams, setTeams] = useState<LinearTeam[]>([]);
  const [statesByTeam, setStatesByTeam] = useState<Record<string, LinearWorkflowState[]>>({});
  // Track which teams we've already kicked off a fetch for so the effect
  // doesn't list `statesByTeam` as a dep (which would re-fire whenever any
  // team's states load). A ref mutation isn't reactive, so the sole input
  // that schedules a fetch is `teamKey`.
  const fetchedTeams = useRef<Set<string>>(new Set());

  useEffect(() => {
    let cancelled = false;
    listLinearTeams()
      .then((res) => {
        if (!cancelled) setTeams(res.teams ?? []);
      })
      .catch(() => {
        if (!cancelled) setTeams([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!teamKey || fetchedTeams.current.has(teamKey)) return;
    fetchedTeams.current.add(teamKey);
    let cancelled = false;
    listLinearStates(teamKey)
      .then((res) => {
        if (!cancelled) setStatesByTeam((prev) => ({ ...prev, [teamKey]: res.states ?? [] }));
      })
      .catch(() => {
        if (!cancelled) setStatesByTeam((prev) => ({ ...prev, [teamKey]: [] }));
      });
    return () => {
      cancelled = true;
    };
  }, [teamKey]);

  const states = teamKey ? (statesByTeam[teamKey] ?? []) : [];
  // "loading" = a team is selected but we haven't received a result yet.
  const loadingStates = !!teamKey && statesByTeam[teamKey] === undefined;
  return { teams, states, loadingStates };
}

function FilterFields({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const { teams, states, loadingStates } = useTeamsAndStates(form.teamKey);
  const toggleState = useCallback(
    (id: string) =>
      setForm((p) => ({
        ...p,
        stateIds: p.stateIds.includes(id)
          ? p.stateIds.filter((s) => s !== id)
          : [...p.stateIds, id],
      })),
    [setForm],
  );

  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label="Team"
          description="Restrict matches to one team."
          value={form.teamKey}
          onChange={(v) => setForm((p) => ({ ...p, teamKey: v, stateIds: [] }))}
          placeholder="(any team)"
          items={teams.map((t) => ({ id: t.key, label: `${t.name} (${t.key})` }))}
        />
        <SelectField
          label="Assignee"
          description="Filter by who an issue is assigned to."
          value={form.assigned || ASSIGNED_ANY}
          onChange={(v) => setForm((p) => ({ ...p, assigned: v === ASSIGNED_ANY ? "" : v }))}
          placeholder="(any)"
          items={[
            { id: ASSIGNED_ANY, label: "(any)" },
            { id: "me", label: "Me" },
            { id: "unassigned", label: "Unassigned" },
          ]}
        />
      </div>
      <div className="space-y-1.5">
        <Label>States</Label>
        <p className="text-xs text-muted-foreground">
          Click states to toggle. Empty matches every state on the team.
        </p>
        <StateMultiSelect
          states={states}
          loading={loadingStates}
          selected={form.stateIds}
          onToggle={toggleState}
          disabled={!form.teamKey}
        />
      </div>
      <div className="space-y-1.5">
        <Label>Query</Label>
        <p className="text-xs text-muted-foreground">
          Free-text match across title and description (optional).
        </p>
        <Input
          value={form.query}
          onChange={(e) => setForm((p) => ({ ...p, query: e.target.value }))}
          placeholder="auth bug"
        />
      </div>
    </>
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
            {LINEAR_ISSUE_WATCH_PLACEHOLDERS.map((p) => (
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

function PromptField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
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
          value={value}
          onChange={onChange}
          language="markdown"
          height={computeEditorHeight(value)}
          lineNumbers="off"
          placeholders={LINEAR_ISSUE_WATCH_PLACEHOLDERS}
        />
      </div>
    </div>
  );
}

function WorkspacePicker({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  return (
    <SelectField
      label="Workspace"
      description="Tasks created by this watcher land in the selected workspace."
      value={value}
      onChange={onChange}
      placeholder="Select workspace"
      items={workspaces.map((w) => ({ id: w.id, label: w.name }))}
      disabled={disabled}
    />
  );
}

function AutomationFields({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const { workflows, agentProfiles, allExecutorProfiles } = useFormData(form.workspaceId);
  const { steps, loading: stepsLoading } = useWorkflowSteps(form.workflowId);
  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label="Workflow"
          description="Tasks are created in this workflow."
          value={form.workflowId}
          onChange={(v) => setForm((p) => ({ ...p, workflowId: v, workflowStepId: "" }))}
          placeholder="Select workflow"
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label="Workflow Step"
          description="Initial step for new tasks."
          value={form.workflowStepId}
          onChange={(v) => setForm((p) => ({ ...p, workflowStepId: v }))}
          placeholder={stepPlaceholder(form.workflowId, stepsLoading, steps.length)}
          items={steps.map((s) => ({ id: s.id, label: s.name }))}
          disabled={!form.workflowId || stepsLoading || steps.length === 0}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label="Agent Profile"
          description="Optional — falls back to step default."
          value={form.agentProfileId}
          onChange={(v) => setForm((p) => ({ ...p, agentProfileId: v }))}
          placeholder="(use step default)"
          items={agentProfiles.map((p) => ({ id: p.id, label: p.label }))}
        />
        <SelectField
          label="Executor Profile"
          description="Optional — falls back to step default."
          value={form.executorProfileId}
          onChange={(v) => setForm((p) => ({ ...p, executorProfileId: v }))}
          placeholder="(use step default)"
          items={allExecutorProfiles.map((p) => ({ id: p.id, label: p.name }))}
        />
      </div>
    </>
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
          How often to re-run the search. Minimum 60s, maximum 3600s.
        </p>
        <Input
          type="number"
          value={form.pollInterval}
          onChange={(e) => setForm((p) => ({ ...p, pollInterval: Number(e.target.value) }))}
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
          onCheckedChange={(v) => setForm((p) => ({ ...p, enabled: v }))}
          className="cursor-pointer"
        />
      </div>
    </>
  );
}

function savingLabel(saving: boolean, isEdit: boolean): string {
  if (saving) return "Saving…";
  return isEdit ? "Update" : "Create";
}

function filterIsEmpty(form: FormState): boolean {
  return (
    form.query.trim() === "" &&
    form.teamKey.trim() === "" &&
    form.assigned.trim() === "" &&
    form.stateIds.length === 0
  );
}

export function LinearIssueWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: Props) {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(() => makeEmptyForm(workspaceId ?? ""));

  useEffect(() => {
    if (watch) {
      setForm(formStateFromWatch(watch));
    } else {
      setForm(makeEmptyForm(workspaceId ?? activeWorkspaceId ?? ""));
    }
  }, [watch, open, workspaceId, activeWorkspaceId]);

  const workspaceLocked = !!watch || !!workspaceId;

  const canSave =
    !!form.workspaceId &&
    !filterIsEmpty(form) &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    !!form.prompt.trim();

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const filter = {
        query: form.query.trim() || undefined,
        teamKey: form.teamKey.trim() || undefined,
        stateIds: form.stateIds.length > 0 ? form.stateIds : undefined,
        assigned: form.assigned.trim() || undefined,
      };
      const payload = {
        filter,
        workflowId: form.workflowId,
        workflowStepId: form.workflowStepId,
        agentProfileId: form.agentProfileId,
        executorProfileId: form.executorProfileId,
        prompt: form.prompt,
        enabled: form.enabled,
        pollIntervalSeconds: form.pollInterval,
      };
      if (watch) {
        await onUpdate(watch.id, payload);
      } else {
        await onCreate({ ...payload, workspaceId: form.workspaceId });
      }
      onOpenChange(false);
    } catch {
      // Error surfaced by caller's toast.
    } finally {
      setSaving(false);
    }
  }, [form, watch, onCreate, onUpdate, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[800px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{watch ? "Edit Linear Watcher" : "Create Linear Watcher"}</DialogTitle>
          <DialogDescription>
            Poll Linear with a structured filter and auto-create a Kandev task for each
            newly-matching issue. Issues are not bound to a repository — the workflow step&apos;s
            defaults decide where the task runs.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <WorkspacePicker
            value={form.workspaceId}
            onChange={(v) =>
              setForm((p) => ({ ...p, workspaceId: v, workflowId: "", workflowStepId: "" }))
            }
            disabled={workspaceLocked}
          />
          <FilterFields form={form} setForm={setForm} />
          <AutomationFields form={form} setForm={setForm} />
          <PromptField
            value={form.prompt}
            onChange={(v) => setForm((p) => ({ ...p, prompt: v }))}
          />
          <SettingsFields form={form} setForm={setForm} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving || !canSave} className="cursor-pointer">
            {savingLabel(saving, !!watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
