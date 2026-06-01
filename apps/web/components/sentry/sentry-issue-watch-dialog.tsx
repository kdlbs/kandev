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
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
  listSentryProjects,
  listSentryOrganizations,
  fetchSentryConfig,
} from "@/lib/api/domains/sentry-api";
import { SENTRY_ISSUE_WATCH_PLACEHOLDERS } from "./sentry-issue-watch-placeholders";
import { LevelMultiSelect, StatusMultiSelect } from "./sentry-issue-watch-multiselect";
import {
  STATS_PERIOD_OPTIONS,
  type FormState,
  type WatchDefaults,
  USE_DEFAULT,
  orgSelectItems,
  projectSelectItems,
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

function OrgProjectRow({
  form,
  setForm,
  projects,
  orgs,
  defaults,
}: {
  form: FormState;
  setForm: FormSetter;
  projects: SentryProject[];
  orgs: string[];
  defaults: WatchDefaults;
}) {
  const onOrgChange = (v: string) => {
    const slug = v === USE_DEFAULT ? defaults.orgSlug : v;
    // The selected project may belong to a different org — clear it so the
    // project dropdown re-picks within the new org.
    setForm((p) => ({ ...p, orgSlug: slug, projectSlug: "" }));
  };
  const onProjectChange = (v: string) => {
    const slug = v === USE_DEFAULT ? defaults.projectSlug : v;
    setForm((p) => ({ ...p, projectSlug: slug }));
  };
  const orgItems = orgSelectItems(orgs, form.orgSlug, defaults.orgSlug);
  const projectItems = projectSelectItems(projects, form.projectSlug, defaults.projectSlug);
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label="Organization slug"
        description="Required — the Sentry org to poll."
        value={form.orgSlug}
        onChange={onOrgChange}
        placeholder={orgItems.length === 0 ? "No organizations available" : "Select organization"}
        items={orgItems}
        disabled={orgItems.length === 0}
      />
      <SelectField
        label="Project slug"
        description="Required — pick a project visible to the saved auth token."
        value={form.projectSlug}
        onChange={onProjectChange}
        placeholder={projectItems.length === 0 ? "No projects available" : "Select project"}
        items={projectItems}
        disabled={projectItems.length === 0}
      />
    </div>
  );
}

function FilterFields({
  form,
  setForm,
  orgs,
  defaults,
}: {
  form: FormState;
  setForm: FormSetter;
  orgs: string[];
  defaults: WatchDefaults;
}) {
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
      <OrgProjectRow
        form={form}
        setForm={setForm}
        projects={projects}
        orgs={orgs}
        defaults={defaults}
      />
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

// useWatchSelectorData loads the org list (for the org dropdown) and the
// install-wide default org/project from the Sentry config. On a fresh create it
// also prefills the form with those defaults so the user need not re-type the
// same slugs. A single config fetch serves both the prefill and the "Use
// default" options.
function useWatchSelectorData(open: boolean, hasWatch: boolean, setForm: FormSetter) {
  const [orgs, setOrgs] = useState<string[]>([]);
  const [defaults, setDefaults] = useState<WatchDefaults>({ orgSlug: "", projectSlug: "" });
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    listSentryOrganizations()
      .then((res) => {
        if (!cancelled) setOrgs((res.organizations ?? []).map((o) => o.slug));
      })
      .catch(() => {
        if (!cancelled) setOrgs([]);
      });
    fetchSentryConfig()
      .then((cfg) => {
        if (cancelled || !cfg) return;
        setDefaults({ orgSlug: cfg.defaultOrgSlug, projectSlug: cfg.defaultProjectSlug });
        if (hasWatch) return;
        setForm((p) => ({
          ...p,
          orgSlug: p.orgSlug || cfg.defaultOrgSlug,
          projectSlug: p.projectSlug || cfg.defaultProjectSlug,
        }));
      })
      .catch(() => {
        /* fall through — user can still pick from the org dropdown */
      });
    return () => {
      cancelled = true;
    };
  }, [open, hasWatch, setForm]);
  return { orgs, defaults };
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

  const { orgs, defaults } = useWatchSelectorData(open, !!watch, setForm);

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
          <FilterFields form={form} setForm={setForm} orgs={orgs} defaults={defaults} />
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
