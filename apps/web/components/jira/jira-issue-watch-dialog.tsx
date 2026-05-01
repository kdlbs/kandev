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
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { searchJiraTickets } from "@/lib/api/domains/jira-api";
import type {
  CreateJiraIssueWatchInput,
  JiraIssueWatch,
  UpdateJiraIssueWatchInput,
} from "@/lib/types/jira";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: JiraIssueWatch | null;
  workspaceId: string;
  onCreate: (req: CreateJiraIssueWatchInput) => Promise<unknown>;
  onUpdate: (id: string, req: UpdateJiraIssueWatchInput) => Promise<unknown>;
};

type FormState = {
  jql: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollInterval: number;
};

const DEFAULT_JQL = `project = PROJ AND status = "Open" ORDER BY created DESC`;
const DEFAULT_PROMPT = `Investigate JIRA ticket {{issue.key}}: {{issue.summary}}\n\n{{issue.url}}`;

const emptyForm: FormState = {
  jql: DEFAULT_JQL,
  workflowId: "",
  workflowStepId: "",
  agentProfileId: "",
  executorProfileId: "",
  prompt: DEFAULT_PROMPT,
  enabled: true,
  pollInterval: 300,
};

function formStateFromWatch(w: JiraIssueWatch): FormState {
  return {
    jql: w.jql,
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
    agentProfileId: w.agentProfileId,
    executorProfileId: w.executorProfileId,
    prompt: w.prompt || DEFAULT_PROMPT,
    enabled: w.enabled,
    pollInterval: w.pollIntervalSeconds,
  };
}

type StepOption = { id: string; name: string };

function useWorkflowSteps(workflowId: string) {
  const [steps, setSteps] = useState<StepOption[]>([]);
  useEffect(() => {
    if (!workflowId) {
      // Defer to next tick so we don't sync-set state inside an effect body.
      void Promise.resolve().then(() => setSteps([]));
      return;
    }
    let cancelled = false;
    listWorkflowSteps(workflowId)
      .then((res) => {
        if (cancelled) return;
        const sorted = [...res.steps].sort((a, b) => a.position - b.position);
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

function JQLField({
  workspaceId,
  jql,
  onChange,
}: {
  workspaceId: string;
  jql: string;
  onChange: (v: string) => void;
}) {
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);
  const [testing, setTesting] = useState(false);
  const handleTest = useCallback(async () => {
    setTesting(true);
    setResult(null);
    try {
      const res = await searchJiraTickets(workspaceId, { jql, maxResults: 5 });
      setResult({ ok: true, message: `Matched ${res.tickets.length} ticket(s) in this page.` });
    } catch (err) {
      setResult({ ok: false, message: `JQL error: ${String(err)}` });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, jql]);

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label>JQL</Label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleTest}
          disabled={!jql.trim() || testing}
          className="cursor-pointer h-7"
        >
          {testing ? "Testing…" : "Test JQL"}
        </Button>
      </div>
      <Textarea
        value={jql}
        onChange={(e) => onChange(e.target.value)}
        placeholder='project = PROJ AND status = "Open"'
        rows={3}
        className="font-mono text-xs resize-y"
      />
      <p className="text-xs text-muted-foreground">
        Atlassian JQL. The watcher polls this query and creates one task per newly-matching ticket.
      </p>
      {result && (
        <p className={`text-xs ${result.ok ? "text-emerald-600" : "text-destructive"}`}>
          {result.message}
        </p>
      )}
    </div>
  );
}

function PromptField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="space-y-1.5">
      <Label>Task Prompt</Label>
      <p className="text-xs text-muted-foreground">
        Sent to the agent for each new ticket. Placeholders: {`{{issue.key}}`},{" "}
        {`{{issue.summary}}`}, {`{{issue.url}}`}, {`{{issue.status}}`}, {`{{issue.priority}}`},{" "}
        {`{{issue.type}}`}, {`{{issue.assignee}}`}, {`{{issue.reporter}}`}, {`{{issue.project}}`},{" "}
        {`{{issue.description}}`}.
      </p>
      <Textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={5}
        className="text-sm resize-y"
      />
    </div>
  );
}

function AutomationFields({
  form,
  setForm,
  workspaceId,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  workspaceId: string;
}) {
  const { workflows, agentProfiles, allExecutorProfiles } = useFormData(workspaceId);
  const steps = useWorkflowSteps(form.workflowId);
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
          placeholder={form.workflowId ? "Select step" : "Select a workflow first"}
          items={steps.map((s) => ({ id: s.id, label: s.name }))}
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
          How often to re-run the JQL. Minimum 60s, maximum 3600s.
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

export function JiraIssueWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: Props) {
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(emptyForm);

  useEffect(() => {
    setForm(watch ? formStateFromWatch(watch) : emptyForm);
  }, [watch, open]);

  const canSave =
    !!form.jql.trim() && !!form.workflowId && !!form.workflowStepId && !!form.prompt.trim();

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const payload = {
        jql: form.jql,
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
        await onCreate({ ...payload, workspaceId });
      }
      onOpenChange(false);
    } catch {
      // Error surfaced by caller's toast.
    } finally {
      setSaving(false);
    }
  }, [form, watch, workspaceId, onCreate, onUpdate, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[800px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{watch ? "Edit JIRA Watcher" : "Create JIRA Watcher"}</DialogTitle>
          <DialogDescription>
            Poll a JQL query and auto-create a Kandev task for each newly-matching ticket. Tickets
            are not bound to a repository — the workflow step&apos;s defaults decide where the task
            runs.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <JQLField
            workspaceId={workspaceId}
            jql={form.jql}
            onChange={(v) => setForm((p) => ({ ...p, jql: v }))}
          />
          <AutomationFields form={form} setForm={setForm} workspaceId={workspaceId} />
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
