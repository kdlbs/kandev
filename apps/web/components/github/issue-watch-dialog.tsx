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
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
  issueWatchPlaceholders,
  DEFAULT_ISSUE_WATCH_PROMPT,
} from "@/components/github/issue-watch-placeholders";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import { STEP_DEFAULT, STEP_DEFAULT_LABEL, resolveProfileId } from "@/lib/watcher-profile-default";
import type {
  RepoFilter,
  IssueWatch,
  CreateIssueWatchRequest,
  UpdateIssueWatchRequest,
  CleanupPolicy,
} from "@/lib/types/github";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  ISSUE_CLEANUP_POLICY_OPTIONS,
  cleanupPolicyDescription,
  cleanupPolicyItems,
} from "./watch-cleanup-policy";

type IssueWatchDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: IssueWatch | null;
  // Pre-binds the dialog to one workspace. Omit on the install-wide settings
  // page so the create flow shows a workspace picker.
  workspaceId?: string;
  onCreate: (req: CreateIssueWatchRequest) => Promise<void>;
  onUpdate: (id: string, req: UpdateIssueWatchRequest) => Promise<void>;
};

type FormState = {
  workspaceId: string;
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
  cleanupPolicy: CleanupPolicy;
};

const DEFAULT_QUERY = "type:issue state:open";

function makeDefaultForm(workspaceId: string): FormState {
  return {
    workspaceId,
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
    cleanupPolicy: "auto",
  };
}

function formStateFromWatch(watch: IssueWatch): FormState {
  const hasRepos = watch.repos && watch.repos.length > 0;
  return {
    workspaceId: watch.workspace_id,
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
    cleanupPolicy: watch.cleanup_policy ?? "auto",
  };
}

function useWatchFormData(workspaceId: string) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);

  const allWorkflows = useAppStore((state) => state.workflows.items);
  const workflows = useMemo(() => allWorkflows.filter((w) => !w.hidden), [allWorkflows]);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const allExecutorProfiles = useMemo(
    () =>
      executors
        .filter((e) => e.type !== "local" && e.type !== "local_pc")
        .flatMap((e) => e.profiles ?? []),
    [executors],
  );

  return { workflows, agentProfiles, allExecutorProfiles };
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
  const { t } = useTranslation();
  const placeholders = useMemo(() => issueWatchPlaceholders(t), [t]);
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">{t("github:availablePlaceholders")}</p>
          <ul className="text-xs space-y-0.5">
            {placeholders.map((p) => (
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
  workspaceLocked,
  onAllReposChange,
  onSelectedReposChange,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  workspaceLocked: boolean;
  onAllReposChange: (checked: boolean) => void;
  onSelectedReposChange: (repos: RepoFilter[]) => void;
}) {
  return (
    <div className="space-y-5">
      <WorkspacePicker
        value={form.workspaceId}
        onChange={(v) =>
          setForm((prev) => ({
            ...prev,
            workspaceId: v,
            workflowId: "",
            workflowStepId: "",
            selectedRepos: [],
            allRepos: false,
          }))
        }
        disabled={workspaceLocked}
      />
      <IssueFilterFields
        form={form}
        setForm={setForm}
        onAllReposChange={onAllReposChange}
        onSelectedReposChange={onSelectedReposChange}
      />
      <IssueAutomationFields form={form} setForm={setForm} />
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
  const { t } = useTranslation();
  return (
    <>
      <SectionHeader>{t("github:filter")}</SectionHeader>
      <RepoFilterSelector
        allRepos={form.allRepos}
        selectedRepos={form.selectedRepos}
        onAllReposChange={onAllReposChange}
        onSelectedReposChange={onSelectedReposChange}
        workspaceId={form.workspaceId}
      />
      <div className="space-y-1.5">
        <Label>{t("github:labelsCommaSeparated")}</Label>
        <Input
          value={form.labels}
          onChange={(e) => setForm((prev) => ({ ...prev, labels: e.target.value }))}
          placeholder={t("github:eGBugEnhancementPriorityHigh")}
        />
        <p className="text-xs text-muted-foreground">
          {t("github:onlyMatchIssuesWithTheseLabels")}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label>{t("github:customQuery")}</Label>
        <Textarea
          value={form.customQuery}
          onChange={(e) => setForm((prev) => ({ ...prev, customQuery: e.target.value }))}
          placeholder={t("github:queryExample", { query: "type:issue state:open label:bug" })}
          rows={1}
          className="font-mono text-xs resize-y"
        />
        <p className="text-xs text-muted-foreground">
          {t("github:githubSearchQueryWhenSetOverrides")}
        </p>
      </div>
    </>
  );
}

function IssueAutomationFields({
  form,
  setForm,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
}) {
  const { t } = useTranslation();
  const { workflows, agentProfiles, allExecutorProfiles } = useWatchFormData(form.workspaceId);
  const { steps: workflowSteps, loading: stepsLoading } = useWorkflowSteps(form.workflowId);
  const placeholders = useMemo(() => issueWatchPlaceholders(t), [t]);

  return (
    <>
      <SectionHeader>{t("github:automation")}</SectionHeader>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("github:workflow")}
          description={t("github:theWorkflowToCreateTasksIn")}
          value={form.workflowId}
          onChange={(v) => setForm((prev) => ({ ...prev, workflowId: v, workflowStepId: "" }))}
          placeholder={t("github:selectWorkflow")}
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label={t("github:workflowStep")}
          description={t("github:initialStepForNewTasks")}
          value={form.workflowStepId}
          onChange={(v) => setForm((prev) => ({ ...prev, workflowStepId: v }))}
          placeholder={stepPlaceholder(form.workflowId, stepsLoading, workflowSteps.length)}
          items={workflowSteps.map((s) => ({ id: s.id, label: s.name }))}
          disabled={!form.workflowId || stepsLoading || workflowSteps.length === 0}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("github:agentProfile")}
          description={t("github:optionalFallsBackToStepDefault")}
          value={form.agentProfileId || STEP_DEFAULT}
          onChange={(v) => setForm((prev) => ({ ...prev, agentProfileId: resolveProfileId(v) }))}
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
          label={t("github:executorProfile")}
          description={t("github:optionalFallsBackToStepDefault")}
          value={form.executorProfileId || STEP_DEFAULT}
          onChange={(v) => setForm((prev) => ({ ...prev, executorProfileId: resolveProfileId(v) }))}
          placeholder={STEP_DEFAULT_LABEL}
          items={[
            { id: STEP_DEFAULT, label: STEP_DEFAULT_LABEL },
            ...allExecutorProfiles.map((p) => ({ id: p.id, label: p.name })),
          ]}
        />
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          <Label>{t("github:taskPrompt")}</Label>
          <PlaceholdersHelp />
        </div>
        <p className="text-xs text-muted-foreground">
          {/* `{{` is passed as a value so it never reaches the catalog, where
              i18next would read it as an interpolation opener. */}
          {t("github:issueWatchPromptHelp", { token: "{{" })}
        </p>
        <div className="rounded-md border border-border overflow-hidden">
          <ScriptEditor
            value={form.prompt}
            onChange={(v) => setForm((prev) => ({ ...prev, prompt: v }))}
            language="markdown"
            height={computeEditorHeight(form.prompt)}
            lineNumbers="off"
            placeholders={placeholders}
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
  const { t } = useTranslation();
  return (
    <>
      <SectionHeader>{t("common:settings")}</SectionHeader>
      <div className="space-y-1.5">
        <Label>{t("github:pollIntervalSeconds")}</Label>
        <p className="text-xs text-muted-foreground">{t("github:howOftenToCheckForNew")}</p>
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
          <Label>{t("github:enabled")}</Label>
          <p className="text-xs text-muted-foreground">{t("github:pauseOrResumePolling")}</p>
        </div>
        <Switch
          checked={form.enabled}
          onCheckedChange={(v) => setForm((prev) => ({ ...prev, enabled: v }))}
          className="cursor-pointer"
        />
      </div>
      <SelectField
        label={t("github:cleanupBehavior")}
        description={cleanupPolicyDescription(t, ISSUE_CLEANUP_POLICY_OPTIONS, form.cleanupPolicy)}
        value={form.cleanupPolicy}
        onChange={(v) => setForm((prev) => ({ ...prev, cleanupPolicy: v as CleanupPolicy }))}
        placeholder={t("github:auto")}
        items={cleanupPolicyItems(t, ISSUE_CLEANUP_POLICY_OPTIONS)}
      />
    </>
  );
}

function parseLabels(labelsStr: string): string[] {
  return labelsStr
    .split(",")
    .map((l) => l.trim())
    .filter((l) => l.length > 0);
}

// Plain function, so `t` is threaded in — the guard never inspects return values.
function getSaveLabel(t: TFunction, watch: IssueWatch | null | undefined): string {
  return watch ? t("github:update") : t("github:create");
}

export function IssueWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: IssueWatchDialogProps) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(() => makeDefaultForm(workspaceId ?? ""));

  useEffect(() => {
    if (watch) {
      setForm(formStateFromWatch(watch));
    } else {
      setForm(makeDefaultForm(workspaceId ?? activeWorkspaceId ?? ""));
    }
  }, [watch, open, workspaceId, activeWorkspaceId]);

  const workspaceLocked = !!watch || !!workspaceId;

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
        cleanup_policy: form.cleanupPolicy,
      };
      if (watch) {
        await onUpdate(watch.id, payload);
      } else {
        await onCreate({ ...payload, workspace_id: form.workspaceId });
      }
      onOpenChange(false);
    } catch {
      // Error handled by caller
    } finally {
      setSaving(false);
    }
  }, [form, watch, onCreate, onUpdate, onOpenChange]);

  const canSave =
    !!form.workspaceId &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    form.prompt.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[900px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {watch ? t("github:editIssueWatch") : t("github:createIssueWatch")}
          </DialogTitle>
          <DialogDescription>{t("github:automaticallyCreateTasksWhenNewGithub")}</DialogDescription>
        </DialogHeader>
        <IssueWatchFormFields
          form={form}
          setForm={setForm}
          workspaceLocked={workspaceLocked}
          onAllReposChange={handleAllReposChange}
          onSelectedReposChange={handleSelectedReposChange}
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving || !canSave} className="cursor-pointer">
            {saving ? t("github:saving") : getSaveLabel(t, watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
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
      description={t("github:tasksCreatedByThisWatcherLand")}
      value={value}
      onChange={onChange}
      placeholder={t("github:selectWorkspace")}
      items={workspaces.map((w) => ({ id: w.id, label: w.name }))}
      disabled={disabled}
    />
  );
}
