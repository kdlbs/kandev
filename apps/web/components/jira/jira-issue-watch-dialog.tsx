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
import { CliModeIcon } from "@/components/cli-mode-icon";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import { searchJiraTickets } from "@/lib/api/domains/jira-api";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
  jiraIssueWatchPlaceholders,
  DEFAULT_JIRA_ISSUE_WATCH_PROMPT,
} from "@/components/jira/jira-issue-watch-placeholders";
import { STEP_DEFAULT, STEP_DEFAULT_LABEL, resolveProfileId } from "@/lib/watcher-profile-default";
import { WatcherRepositoryFields } from "@/components/watcher-repository-fields";
import { clearWorkspaceScopedForm } from "@/lib/watcher-repository-default";
import type {
  CreateJiraIssueWatchInput,
  JiraIssueWatch,
  UpdateJiraIssueWatchInput,
} from "@/lib/types/jira";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: JiraIssueWatch | null;
  // workspaceId pre-binds the dialog to one workspace (legacy single-workspace
  // surfaces). Omit on the install-wide settings page so the create dialog
  // shows a workspace picker; when editing existing watches, the form pulls
  // workspaceId from the watch row regardless.
  workspaceId?: string;
  onCreate: (req: CreateJiraIssueWatchInput) => Promise<unknown>;
  onUpdate: (id: string, req: UpdateJiraIssueWatchInput) => Promise<unknown>;
};

type FormState = {
  workspaceId: string;
  jql: string;
  workflowId: string;
  workflowStepId: string;
  /** Optional repository binding; "" = unbound (repo-less task). */
  repositoryId: string;
  /** Base branch for the worktree; "" = the repository's default branch. */
  baseBranch: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollInterval: number;
  /**
   * Per-watcher throttle cap as a free-text input: empty string means
   * "uncapped" (sent as null), non-empty must parse to a positive integer.
   */
  maxInflightTasks: string;
};

function maxInflightTasksString(v: number | null | undefined): string {
  if (v === undefined || v === null) return "";
  if (!Number.isFinite(v) || v <= 0) return "";
  return String(v);
}

function parseMaxInflightTasks(raw: string): number | null | "invalid" {
  const t = raw.trim();
  if (t === "") return null;
  const n = Number(t);
  if (!Number.isInteger(n) || n <= 0) return "invalid";
  return n;
}

// JQL, not copy. Every token here is parsed by Jira — field names (`project`,
// `status`, `created`), operators and a project key — and the string is
// persisted on the watch and sent upstream verbatim. Translating any of it would
// produce a query Jira rejects.
const DEFAULT_JQL = `project = PROJ AND status = "Open" ORDER BY created DESC`;

function makeEmptyForm(workspaceId: string): FormState {
  return {
    workspaceId,
    jql: DEFAULT_JQL,
    workflowId: "",
    workflowStepId: "",
    repositoryId: "",
    baseBranch: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: DEFAULT_JIRA_ISSUE_WATCH_PROMPT,
    enabled: true,
    pollInterval: 300,
    maxInflightTasks: "5",
  };
}

function formStateFromWatch(w: JiraIssueWatch): FormState {
  return {
    workspaceId: w.workspaceId,
    jql: w.jql,
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
    repositoryId: w.repositoryId ?? "",
    baseBranch: w.baseBranch ?? "",
    agentProfileId: w.agentProfileId,
    executorProfileId: w.executorProfileId,
    prompt: w.prompt.trim() ? w.prompt : DEFAULT_JIRA_ISSUE_WATCH_PROMPT,
    enabled: w.enabled,
    pollInterval: w.pollIntervalSeconds,
    maxInflightTasks: maxInflightTasksString(w.maxInflightTasks),
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

function JQLField({
  workspaceId,
  jql,
  onChange,
}: {
  workspaceId: string;
  jql: string;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);
  const [testing, setTesting] = useState(false);
  const handleTest = useCallback(async () => {
    setTesting(true);
    setResult(null);
    try {
      const res = await searchJiraTickets({ jql, maxResults: 5 }, { workspaceId });
      // "ticket(s)" was an English plural hack; the count now selects the form.
      setResult({ ok: true, message: t("jira:matchedTickets", { count: res.tickets.length }) });
    } catch (err) {
      setResult({ ok: false, message: t("jira:jqlError", { error: String(err) }) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, jql, t]);

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
          {testing ? t("jira:testingJql") : t("jira:testJql")}
        </Button>
      </div>
      <Textarea
        value={jql}
        onChange={(e) => onChange(e.target.value)}
        // A sample JQL query. JQL is a query language, not copy: `project`,
        // `status` and `PROJ` are the field names and project key Jira parses,
        // so a translated (or pseudo-transliterated) example would not run.
        placeholder='project = PROJ AND status = "Open"'
        rows={3}
        className="font-mono text-xs resize-y"
      />
      <p className="text-xs text-muted-foreground">{t("jira:atlassianJqlHelp")}</p>
      {result && (
        <p className={`text-xs ${result.ok ? "text-emerald-600" : "text-destructive"}`}>
          {result.message}
        </p>
      )}
    </div>
  );
}

function PlaceholdersHelp() {
  const { t } = useTranslation();
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">{t("jira:availablePlaceholders")}</p>
          <ul className="text-xs space-y-0.5">
            {jiraIssueWatchPlaceholders(t).map((p) => (
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
  const { t } = useTranslation();
  // Memoized because `ScriptEditor` keys its Monaco completion-provider
  // registration on `placeholders` identity. This used to be a module-scope
  // const, so it was stable for free; now that it is built from `t`, a fresh
  // array on every render would re-register the provider on every keystroke.
  const placeholders = useMemo(() => jiraIssueWatchPlaceholders(t), [t]);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>{t("jira:taskPrompt")}</Label>
        <PlaceholdersHelp />
      </div>
      <p className="text-xs text-muted-foreground">
        {/* The `{{` token is passed as a value so it never reaches the catalog,
            where i18next would interpolate it away. */}
        {t("jira:promptFieldHelp", { token: "{{" })}
      </p>
      <div className="rounded-md border border-border overflow-hidden">
        <ScriptEditor
          value={value}
          onChange={onChange}
          language="markdown"
          height={computeEditorHeight(value)}
          lineNumbers="off"
          placeholders={placeholders}
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
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  return (
    <SelectField
      label={t("common:workspace")}
      description={t("jira:tasksCreatedByThisWatcherLand")}
      value={value}
      onChange={onChange}
      placeholder={t("jira:selectWorkspace")}
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
  const { t } = useTranslation();
  const { workflows, agentProfiles, allExecutorProfiles } = useFormData(form.workspaceId);
  const { steps, loading: stepsLoading } = useWorkflowSteps(form.workflowId);
  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("jira:workflow")}
          description={t("jira:tasksAreCreatedInThisWorkflow")}
          value={form.workflowId}
          onChange={(v) => setForm((p) => ({ ...p, workflowId: v, workflowStepId: "" }))}
          placeholder={t("jira:selectWorkflow")}
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label={t("jira:workflowStep")}
          description={t("jira:initialStepForNewTasks")}
          value={form.workflowStepId}
          onChange={(v) => setForm((p) => ({ ...p, workflowStepId: v }))}
          placeholder={stepPlaceholder(form.workflowId, stepsLoading, steps.length)}
          items={steps.map((s) => ({ id: s.id, label: s.name }))}
          disabled={!form.workflowId || stepsLoading || steps.length === 0}
        />
      </div>
      <WatcherRepositoryFields
        workspaceId={form.workspaceId}
        repositoryId={form.repositoryId}
        baseBranch={form.baseBranch}
        onRepositoryChange={(repositoryId) =>
          setForm((p) => ({ ...p, repositoryId, baseBranch: "" }))
        }
        onBaseBranchChange={(baseBranch) => setForm((p) => ({ ...p, baseBranch }))}
      />
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("jira:agentProfile")}
          description={t("jira:optionalFallsBackToStepDefault")}
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
          label={t("jira:executorProfile")}
          description={t("jira:optionalFallsBackToStepDefault")}
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
  const { t } = useTranslation();
  const parsed = parseMaxInflightTasks(form.maxInflightTasks);
  const invalid = parsed === "invalid";
  return (
    <div className="space-y-1.5">
      <Label>{t("jira:maxInFlightTasks")}</Label>
      <p className="text-xs text-muted-foreground">{t("jira:capOnOpenTasksCreatedBy")}</p>
      <Input
        type="number"
        value={form.maxInflightTasks}
        onChange={(e) => setForm((p) => ({ ...p, maxInflightTasks: e.target.value }))}
        min={1}
        step={1}
        placeholder={t("jira:noCap")}
        aria-invalid={invalid}
      />
      {invalid && (
        <p className="text-xs text-destructive">{t("jira:enterAPositiveIntegerOrLeave")}</p>
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
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-1.5">
        <Label>{t("jira:pollIntervalSeconds")}</Label>
        <p className="text-xs text-muted-foreground">{t("jira:howOftenToReRunThe")}</p>
        <Input
          type="number"
          value={form.pollInterval}
          onChange={(e) => setForm((p) => ({ ...p, pollInterval: Number(e.target.value) }))}
          min={60}
          max={3600}
        />
      </div>
      <MaxInflightTasksField form={form} setForm={setForm} />
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("jira:enabled")}</Label>
          <p className="text-xs text-muted-foreground">{t("jira:pauseOrResumePolling")}</p>
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

// `t` is threaded in: this is a plain function, and the guard only inspects JSX,
// so a literal returned from here would never be reported.
function savingLabel(t: TFunction, saving: boolean, isEdit: boolean): string {
  if (saving) return t("jira:saving");
  return isEdit ? t("jira:update") : t("jira:create");
}

export function JiraIssueWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: Props) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(() => makeEmptyForm(workspaceId ?? ""));

  // Re-seed the form whenever the dialog opens or the bound watch changes.
  // Default the workspace to: editing → the watch's workspace; creating →
  // the explicit `workspaceId` prop, falling back to the user's currently
  // active workspace so the picker isn't empty when nothing was preselected.
  useEffect(() => {
    if (watch) {
      setForm(formStateFromWatch(watch));
    } else {
      setForm(makeEmptyForm(workspaceId ?? activeWorkspaceId ?? ""));
    }
  }, [watch, open, workspaceId, activeWorkspaceId]);

  // Workspace is locked when editing (changing it would orphan the workflow
  // refs) or when the dialog was opened from a single-workspace surface.
  const workspaceLocked = !!watch || !!workspaceId;

  const parsedMaxInflight = parseMaxInflightTasks(form.maxInflightTasks);
  const canSave =
    !!form.workspaceId &&
    !!form.jql.trim() &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    !!form.prompt.trim() &&
    parsedMaxInflight !== "invalid";

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const maxInflight = parseMaxInflightTasks(form.maxInflightTasks);
      if (maxInflight === "invalid") {
        // canSave gates the button but guard the handler too — see Linear dialog.
        return;
      }
      const payload = {
        jql: form.jql,
        workflowId: form.workflowId,
        workflowStepId: form.workflowStepId,
        // Empty repositoryId clears the binding; empty base branch is sent
        // verbatim so the backend fills the repo's default at save time.
        repositoryId: form.repositoryId,
        baseBranch: form.repositoryId ? form.baseBranch : "",
        agentProfileId: form.agentProfileId,
        executorProfileId: form.executorProfileId,
        prompt: form.prompt,
        enabled: form.enabled,
        pollIntervalSeconds: form.pollInterval,
        maxInflightTasks: maxInflight,
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
          <DialogTitle>
            {watch ? t("jira:editJiraWatcher") : t("jira:createJiraWatcher")}
          </DialogTitle>
          <DialogDescription>{t("jira:watchDialogDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <WorkspacePicker
            value={form.workspaceId}
            onChange={(v) => setForm((p) => clearWorkspaceScopedForm(p, v))}
            disabled={workspaceLocked}
          />
          <JQLField
            workspaceId={form.workspaceId}
            jql={form.jql}
            onChange={(v) => setForm((p) => ({ ...p, jql: v }))}
          />
          <AutomationFields form={form} setForm={setForm} />
          <PromptField
            value={form.prompt}
            onChange={(v) => setForm((p) => ({ ...p, prompt: v }))}
          />
          <SettingsFields form={form} setForm={setForm} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving || !canSave} className="cursor-pointer">
            {savingLabel(t, saving, !!watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
