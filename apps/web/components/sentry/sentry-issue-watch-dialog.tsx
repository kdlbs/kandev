"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
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
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import { listSentryProjects, fetchSentryConfig } from "@/lib/api/domains/sentry-api";
import { SENTRY_ISSUE_WATCH_PLACEHOLDERS } from "./sentry-issue-watch-placeholders";
import { levelBadgeClass, statusBadgeClass } from "./sentry-issue-common";
import {
  LEVEL_OPTIONS,
  STATUS_OPTIONS,
  STATS_PERIOD_OPTIONS,
  type FormState,
  buildFilterPayload,
  formStateFromWatch,
  makeEmptyForm,
} from "./sentry-issue-watch-form";
import type {
  CreateSentryIssueWatchRequest,
  SentryIssueWatch,
  SentryLevel,
  SentryProject,
  SentryStatus,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: SentryIssueWatch | null;
  workspaceId?: string;
  onCreate: (req: CreateSentryIssueWatchRequest) => Promise<unknown>;
  onUpdate: (
    id: string,
    workspaceId: string,
    req: UpdateSentryIssueWatchRequest,
  ) => Promise<unknown>;
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
  const filteredAgentProfiles = useMemo(
    () => agentProfiles.filter((p) => !p.cli_passthrough),
    [agentProfiles],
  );
  return { workflows, agentProfiles: filteredAgentProfiles, allExecutorProfiles };
}

function useSentryProjects(orgSlug: string) {
  const [projects, setProjects] = useState<SentryProject[]>([]);
  useEffect(() => {
    let cancelled = false;
    listSentryProjects()
      .then((res) => {
        if (!cancelled) setProjects(res.projects ?? []);
      })
      .catch(() => {
        if (!cancelled) setProjects([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);
  // Sentry's auth-token endpoint already filters to the user's accessible orgs;
  // if an orgSlug is set, restrict to projects that match.
  return useMemo(
    () => (orgSlug ? projects.filter((p) => p.orgSlug === orgSlug) : projects),
    [projects, orgSlug],
  );
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

type FormSetter = React.Dispatch<React.SetStateAction<FormState>>;

function LevelMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryLevel[];
  onToggle: (level: SentryLevel) => void;
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {LEVEL_OPTIONS.map((level) => {
        const active = selected.includes(level);
        const colorClass = active ? levelBadgeClass(level) : "";
        return (
          <button
            key={level}
            type="button"
            onClick={() => onToggle(level)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant="outline" className={`uppercase ${colorClass}`}>
              {level}
            </Badge>
          </button>
        );
      })}
    </div>
  );
}

function StatusMultiSelect({
  selected,
  onToggle,
}: {
  selected: SentryStatus[];
  onToggle: (status: SentryStatus) => void;
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {STATUS_OPTIONS.map((status) => {
        const active = selected.includes(status);
        const colorClass = active ? statusBadgeClass(status) : "";
        return (
          <button
            key={status}
            type="button"
            onClick={() => onToggle(status)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant="outline" className={`uppercase ${colorClass}`}>
              {status}
            </Badge>
          </button>
        );
      })}
    </div>
  );
}

function OrgProjectRow({
  form,
  setForm,
  projects,
}: {
  form: FormState;
  setForm: FormSetter;
  projects: SentryProject[];
}) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <Label>Organization slug</Label>
        <p className="text-xs text-muted-foreground">Required — the Sentry org to poll.</p>
        <Input
          value={form.orgSlug}
          onChange={(e) => setForm((p) => ({ ...p, orgSlug: e.target.value, projectSlug: "" }))}
          placeholder="my-org"
        />
      </div>
      <SelectField
        label="Project slug"
        description="Required — pick a project visible to the saved auth token."
        value={form.projectSlug}
        onChange={(v) => setForm((p) => ({ ...p, projectSlug: v }))}
        placeholder={projects.length === 0 ? "No projects available" : "Select project"}
        items={projects.map((p) => ({ id: p.slug, label: `${p.name} (${p.slug})` }))}
        disabled={projects.length === 0}
      />
    </div>
  );
}

function FilterFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const projects = useSentryProjects(form.orgSlug);
  const toggleLevel = useCallback(
    (level: SentryLevel) =>
      setForm((p) => ({
        ...p,
        levels: p.levels.includes(level)
          ? p.levels.filter((l) => l !== level)
          : [...p.levels, level],
      })),
    [setForm],
  );
  const toggleStatus = useCallback(
    (status: SentryStatus) =>
      setForm((p) => ({
        ...p,
        statuses: p.statuses.includes(status)
          ? p.statuses.filter((s) => s !== status)
          : [...p.statuses, status],
      })),
    [setForm],
  );
  return (
    <>
      <OrgProjectRow form={form} setForm={setForm} projects={projects} />
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label>Environment</Label>
          <p className="text-xs text-muted-foreground">Optional — restrict to one environment.</p>
          <Input
            value={form.environment}
            onChange={(e) => setForm((p) => ({ ...p, environment: e.target.value }))}
            placeholder="production"
          />
        </div>
        <SelectField
          label="Stats period"
          description="How far back to look for matching issues."
          value={form.statsPeriod}
          onChange={(v) => setForm((p) => ({ ...p, statsPeriod: v }))}
          placeholder="(any)"
          items={STATS_PERIOD_OPTIONS.map((o) => ({ id: o.value, label: o.label }))}
        />
      </div>
      <div className="space-y-1.5">
        <Label>Levels</Label>
        <p className="text-xs text-muted-foreground">
          Click to toggle. Matches issues at ANY of the selected levels.
        </p>
        <LevelMultiSelect selected={form.levels} onToggle={toggleLevel} />
      </div>
      <div className="space-y-1.5">
        <Label>Statuses</Label>
        <p className="text-xs text-muted-foreground">
          Click to toggle. Matches issues at ANY of the selected statuses.
        </p>
        <StatusMultiSelect selected={form.statuses} onToggle={toggleStatus} />
      </div>
      <div className="space-y-1.5">
        <Label>Query</Label>
        <p className="text-xs text-muted-foreground">Free-text Sentry search query (optional).</p>
        <Input
          value={form.query}
          onChange={(e) => setForm((p) => ({ ...p, query: e.target.value }))}
          placeholder="is:unresolved transaction:/api/checkout"
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
            {SENTRY_ISSUE_WATCH_PLACEHOLDERS.map((p) => (
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
          placeholders={SENTRY_ISSUE_WATCH_PLACEHOLDERS}
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

function AutomationFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
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

function SettingsFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
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

// Prefill the form on first open with the default org/project from the
// install-wide Sentry config — saves the user from re-typing the same slugs.
function useConfigDefaults(open: boolean, hasWatch: boolean, setForm: FormSetter) {
  useEffect(() => {
    if (!open || hasWatch) return;
    let cancelled = false;
    fetchSentryConfig()
      .then((cfg) => {
        if (cancelled || !cfg) return;
        setForm((p) => ({
          ...p,
          orgSlug: p.orgSlug || cfg.defaultOrgSlug,
          projectSlug: p.projectSlug || cfg.defaultProjectSlug,
        }));
      })
      .catch(() => {
        /* fall through — user can fill in manually */
      });
    return () => {
      cancelled = true;
    };
  }, [open, hasWatch, setForm]);
}

export function SentryIssueWatchDialog({
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

  useConfigDefaults(open, !!watch, setForm);

  const workspaceLocked = !!watch || !!workspaceId;

  const canSave =
    !!form.workspaceId &&
    !!form.orgSlug.trim() &&
    !!form.projectSlug.trim() &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    !!form.prompt.trim() &&
    Number.isInteger(form.pollInterval) &&
    form.pollInterval >= 60 &&
    form.pollInterval <= 3600;

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const filter = buildFilterPayload(form);
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
        await onUpdate(watch.id, watch.workspaceId, payload);
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
          <DialogTitle>{watch ? "Edit Sentry Watcher" : "Create Sentry Watcher"}</DialogTitle>
          <DialogDescription>
            Poll Sentry with a structured filter and auto-create a Kandev task for each
            newly-matching issue. The workflow step&apos;s defaults decide where the task runs.
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
