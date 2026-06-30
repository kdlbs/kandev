"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
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
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { CliModeIcon } from "@/components/cli-mode-icon";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
  LabelMultiSelect,
  PriorityMultiSelect,
  SortByField,
  StateMultiSelect,
  useTeamsAndStates,
} from "./linear-issue-watch-fields";
import { LINEAR_ISSUE_WATCH_PLACEHOLDERS } from "./linear-issue-watch-placeholders";
import { STEP_DEFAULT, STEP_DEFAULT_LABEL, resolveProfileId } from "@/lib/watcher-profile-default";
import {
  ASSIGNED_ANY,
  CREATOR_ANY,
  type FormState,
  type LinearPriority,
  buildWatchPayload,
  creatorPlaceholder,
  formStateFromWatch,
  isWatchFormReady,
  makeEmptyForm,
  parseMaxInflightTasks,
  userOptionLabel,
} from "./linear-issue-watch-form";
import type {
  CreateLinearIssueWatchInput,
  LinearIssueWatch,
  LinearTeam,
  LinearUser,
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

function useFormData(workspaceId: string) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);
  const allWorkflows = useAppStore((s) => s.workflows.items);
  const workflows = useMemo(() => allWorkflows.filter((w) => !w.hidden), [allWorkflows]);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const executors = useAppStore((s) => s.executors.items);
  const allExecutorProfiles = useMemo(
    () =>
      executors
        .filter((e) => e.type !== "local" && e.type !== "local_pc")
        .flatMap((e) => e.profiles ?? []),
    [executors],
  );
  return { workflows, agentProfiles, allExecutorProfiles };
}

type SelectFieldItem = { id: string; label: string; icon?: React.ReactNode };

function SelectField(props: {
  label: string;
  description?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: SelectFieldItem[];
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
              {item.icon ? (
                <span className="flex items-center gap-1.5">
                  <span>{item.label}</span>
                  {item.icon}
                </span>
              ) : (
                item.label
              )}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function FilterFields({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const { teams, states, labels, users, loadingStates, loadingLabels, loadingUsers } =
    useTeamsAndStates(form.teamKey);
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
  const toggleLabel = useCallback(
    (id: string) =>
      setForm((p) => ({
        ...p,
        labelIds: p.labelIds.includes(id)
          ? p.labelIds.filter((l) => l !== id)
          : [...p.labelIds, id],
      })),
    [setForm],
  );
  const togglePriority = useCallback(
    (priority: LinearPriority) =>
      setForm((p) => ({
        ...p,
        priorities: p.priorities.includes(priority)
          ? p.priorities.filter((x) => x !== priority)
          : [...p.priorities, priority],
      })),
    [setForm],
  );

  return (
    <>
      <TeamRow form={form} setForm={setForm} teams={teams} />
      <AssigneeAndCreatorRow
        form={form}
        setForm={setForm}
        users={users}
        loadingUsers={loadingUsers}
      />
      <div className="space-y-1.5">
        <Label>Priority</Label>
        <p className="text-xs text-muted-foreground">
          Click to toggle. Matches issues at ANY of the selected priorities.
        </p>
        <PriorityMultiSelect selected={form.priorities} onToggle={togglePriority} />
      </div>
      <div className="space-y-1.5">
        <Label>States</Label>
        <p className="text-xs text-muted-foreground">
          {form.teamKey
            ? "Click states to toggle. Empty matches every state on the team."
            : "Pick a team to choose specific workflow states."}
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
        <Label>Labels</Label>
        <p className="text-xs text-muted-foreground">
          {form.teamKey
            ? "Click to toggle. Matches ANY of the selected labels."
            : "Pick a team to choose specific labels."}
        </p>
        <LabelMultiSelect
          labels={labels}
          loading={loadingLabels}
          selected={form.labelIds}
          onToggle={toggleLabel}
          disabled={!form.teamKey}
        />
      </div>
      <EstimateRow form={form} setForm={setForm} />
      <QueryField form={form} setForm={setForm} />
    </>
  );
}

type FormSetter = React.Dispatch<React.SetStateAction<FormState>>;

function TeamRow({
  form,
  setForm,
  teams,
}: {
  form: FormState;
  setForm: FormSetter;
  teams: LinearTeam[];
}) {
  return (
    <SelectField
      label="Team"
      description="Restrict matches to one team."
      value={form.teamKey}
      onChange={(v) =>
        setForm((p) => ({ ...p, teamKey: v, stateIds: [], labelIds: [], creatorId: "" }))
      }
      placeholder="(any team)"
      items={teams.map((t) => ({ id: t.key, label: `${t.name} (${t.key})` }))}
    />
  );
}

function AssigneeAndCreatorRow({
  form,
  setForm,
  users,
  loadingUsers,
}: {
  form: FormState;
  setForm: FormSetter;
  users: LinearUser[];
  loadingUsers: boolean;
}) {
  return (
    <div className="grid grid-cols-2 gap-4">
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
      <SelectField
        label="Creator"
        description="Match issues created by one user."
        value={form.creatorId || CREATOR_ANY}
        onChange={(v) => setForm((p) => ({ ...p, creatorId: v === CREATOR_ANY ? "" : v }))}
        placeholder={creatorPlaceholder(form.teamKey, loadingUsers)}
        items={[
          { id: CREATOR_ANY, label: "(any)" },
          ...users.map((u) => ({ id: u.id, label: userOptionLabel(u) })),
        ]}
        disabled={!form.teamKey || loadingUsers}
      />
    </div>
  );
}

function EstimateRow({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <Label>Estimate min</Label>
        <p className="text-xs text-muted-foreground">Lower bound in points (optional).</p>
        <Input
          type="number"
          value={form.estimateMin}
          onChange={(e) => setForm((p) => ({ ...p, estimateMin: e.target.value }))}
          min={0}
          step="0.5"
          placeholder="e.g. 1"
        />
      </div>
      <div className="space-y-1.5">
        <Label>Estimate max</Label>
        <p className="text-xs text-muted-foreground">Upper bound in points (optional).</p>
        <Input
          type="number"
          value={form.estimateMax}
          onChange={(e) => setForm((p) => ({ ...p, estimateMax: e.target.value }))}
          min={0}
          step="0.5"
          placeholder="e.g. 5"
        />
      </div>
    </div>
  );
}

function QueryField({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  return (
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
          value={form.agentProfileId || STEP_DEFAULT}
          onChange={(v) => setForm((p) => ({ ...p, agentProfileId: resolveProfileId(v) }))}
          placeholder={STEP_DEFAULT_LABEL}
          items={[
            { id: STEP_DEFAULT, label: STEP_DEFAULT_LABEL },
            ...agentProfiles.map((p) => ({
              id: p.id,
              label: p.label,
              icon: p.cli_passthrough ? <CliModeIcon /> : undefined,
            })),
          ]}
        />
        <SelectField
          label="Executor Profile"
          description="Optional — falls back to step default."
          value={form.executorProfileId || STEP_DEFAULT}
          onChange={(v) => setForm((p) => ({ ...p, executorProfileId: resolveProfileId(v) }))}
          placeholder={STEP_DEFAULT_LABEL}
          items={[
            { id: STEP_DEFAULT, label: STEP_DEFAULT_LABEL },
            ...allExecutorProfiles.map((p) => ({ id: p.id, label: p.name })),
          ]}
        />
      </div>
    </>
  );
}

function MaxInflightTasksField({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const parsed = parseMaxInflightTasks(form.maxInflightTasks);
  const invalid = parsed === "invalid";
  return (
    <div className="space-y-1.5">
      <Label>Max in-flight tasks</Label>
      <p className="text-xs text-muted-foreground">
        Cap on open tasks created by this watcher. Leave blank for no cap. New matches are deferred
        to the next poll when the cap is reached.
      </p>
      <Input
        type="number"
        value={form.maxInflightTasks}
        onChange={(e) => setForm((p) => ({ ...p, maxInflightTasks: e.target.value }))}
        min={1}
        step={1}
        placeholder="(no cap)"
        aria-invalid={invalid}
      />
      {invalid && (
        <p className="text-xs text-destructive">Enter a positive integer or leave blank.</p>
      )}
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
      <MaxInflightTasksField form={form} setForm={setForm} />
      <SortByField form={form} setForm={setForm} />
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
  const canSave = isWatchFormReady(form);

  const handleSave = useCallback(async () => {
    const payload = buildWatchPayload(form);
    if (!payload) return; // re-checks the cap input — see canSave gate
    setSaving(true);
    try {
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
          {/* Hairlines separate the five conceptual blocks (Destination /
              Filter / Automation / Prompt / Settings). Each block answers a
              different question, so a consistent rhythm helps users navigate
              the form visually instead of reading it as one long stack. */}
          <Separator />
          <FilterFields form={form} setForm={setForm} />
          <Separator />
          <AutomationFields form={form} setForm={setForm} />
          <Separator />
          <PromptField
            value={form.prompt}
            onChange={(v) => setForm((p) => ({ ...p, prompt: v }))}
          />
          <Separator />
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
