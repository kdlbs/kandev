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
import {
  useWorkflowSteps,
  stepPlaceholder,
  type WorkflowStepOption,
} from "@/hooks/use-workflow-steps";
import { DEFAULT_REVIEW_WATCH_PROMPT } from "@/components/github/review-watch-placeholders";
import { ReviewWatchPromptField } from "@/components/github/review-watch-prompt-field";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import { STEP_DEFAULT, STEP_DEFAULT_LABEL, resolveProfileId } from "@/lib/watcher-profile-default";
import type {
  RepoFilter,
  ReviewWatch,
  CreateReviewWatchRequest,
  UpdateReviewWatchRequest,
  CleanupPolicy,
} from "@/lib/types/github";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  REVIEW_CLEANUP_POLICY_OPTIONS,
  cleanupPolicyDescription,
  cleanupPolicyItems,
} from "./watch-cleanup-policy";

type ReviewWatchDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: ReviewWatch | null;
  // Pre-binds the dialog to one workspace; omit on the install-wide settings
  // page so the create flow shows a workspace picker.
  workspaceId?: string;
  onCreate: (req: CreateReviewWatchRequest) => Promise<void>;
  onUpdate: (id: string, req: UpdateReviewWatchRequest) => Promise<void>;
};

const QUERY_TEMPLATES = {
  meAndTeams: "type:pr state:open review-requested:@me -is:draft",
  me: "type:pr state:open user-review-requested:@me -is:draft",
} as const;

type FormState = {
  workspaceId: string;
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
  cleanupPolicy: CleanupPolicy;
};

function makeDefaultForm(workspaceId: string): FormState {
  return {
    workspaceId,
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
    cleanupPolicy: "auto",
  };
}

function formStateFromWatch(watch: ReviewWatch): FormState {
  const hasRepos = watch.repos && watch.repos.length > 0;
  return {
    workspaceId: watch.workspace_id,
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
    cleanupPolicy: watch.cleanup_policy ?? "auto",
  };
}

// --- Generic select field with description ---

type SelectFieldItem = { id: string; label: string; icon?: React.ReactNode };

type SelectFieldProps = {
  label: string;
  description?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: SelectFieldItem[];
  disabled?: boolean;
};

function SelectField({
  label,
  description,
  value,
  onChange,
  placeholder,
  items,
  disabled,
}: SelectFieldProps) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      <Select value={value || undefined} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
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

// --- Hooks ---

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
  const { t } = useTranslation();
  const current = form.customQuery.trim();
  const isMeAndTeams = current === QUERY_TEMPLATES.meAndTeams;
  const isMe = current === QUERY_TEMPLATES.me;

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label>{t("github:searchQuery")}</Label>
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
                  {t("github:meMyTeams")}
                </Button>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs text-xs">
                {t("github:prsWhereYouOrAnyOf")}
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
                  {t("github:me")}
                </Button>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs text-xs">
                {t("github:onlyPrsWhereYouAreExplicitly")}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>
      <Textarea
        value={form.customQuery}
        onChange={(e) => setForm((prev) => ({ ...prev, customQuery: e.target.value }))}
        placeholder={t("github:queryExample", { query: "type:pr state:open review-requested:@me" })}
        rows={1}
        className="font-mono text-xs resize-y"
      />
      <p className="text-xs text-muted-foreground">
        {t("github:githubSearchQuerySupportsFullGithub")}
      </p>
    </div>
  );
}

function WatchFormFields({
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
  const { t } = useTranslation();
  const { workflows, agentProfiles, allExecutorProfiles } = useWatchFormData(form.workspaceId);
  const { steps: workflowSteps, loading: stepsLoading } = useWorkflowSteps(form.workflowId);

  const handleWorkflowChange = useCallback(
    (v: string) => {
      setForm((prev) => ({ ...prev, workflowId: v, workflowStepId: "" }));
    },
    [setForm],
  );

  return (
    <div className="space-y-5">
      <WorkspacePicker
        value={form.workspaceId}
        onChange={(v) =>
          setForm((prev) => ({
            ...prev,
            workspaceId: v,
            // Switching workspace invalidates workflow/step refs since those
            // are workspace-scoped IDs.
            workflowId: "",
            workflowStepId: "",
            // Repo selections are tied to the workspace's repository list too.
            selectedRepos: [],
            allRepos: false,
          }))
        }
        disabled={workspaceLocked}
      />
      <SectionHeader>{t("github:filter")}</SectionHeader>
      <RepoFilterSelector
        allRepos={form.allRepos}
        selectedRepos={form.selectedRepos}
        onAllReposChange={onAllReposChange}
        onSelectedReposChange={onSelectedReposChange}
        workspaceId={form.workspaceId}
      />
      <QueryField form={form} setForm={setForm} />
      <SectionHeader>{t("github:automation")}</SectionHeader>
      <WorkflowFields
        form={form}
        setForm={setForm}
        workflows={workflows}
        workflowSteps={workflowSteps}
        stepsLoading={stepsLoading}
        onWorkflowChange={handleWorkflowChange}
      />
      <ProfileFields
        form={form}
        setForm={setForm}
        agentProfiles={agentProfiles}
        executorProfiles={allExecutorProfiles}
      />
      <ReviewWatchPromptField
        value={form.prompt}
        onChange={(v) => setForm((prev) => ({ ...prev, prompt: v }))}
      />
      <SectionHeader>{t("common:settings")}</SectionHeader>
      <SettingsFields form={form} setForm={setForm} />
    </div>
  );
}

function WorkflowFields({
  form,
  setForm,
  workflows,
  workflowSteps,
  stepsLoading,
  onWorkflowChange,
}: {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  workflows: Array<{ id: string; name: string }>;
  workflowSteps: WorkflowStepOption[];
  stepsLoading: boolean;
  onWorkflowChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label={t("github:workflow")}
        description={t("github:theWorkflowToCreateTasksIn")}
        value={form.workflowId}
        onChange={onWorkflowChange}
        placeholder={t("github:selectWorkflow")}
        items={workflows.map((w) => ({ id: w.id, label: w.name }))}
      />
      <SelectField
        label={t("github:workflowStep")}
        description={t("github:initialStepForNewTasksAuto")}
        value={form.workflowStepId}
        onChange={(v) => setForm((prev) => ({ ...prev, workflowStepId: v }))}
        placeholder={stepPlaceholder(form.workflowId, stepsLoading, workflowSteps.length)}
        items={workflowSteps.map((s) => ({ id: s.id, label: s.name }))}
        disabled={!form.workflowId || stepsLoading || workflowSteps.length === 0}
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
  agentProfiles: Array<{ id: string; label: string; cli_passthrough?: boolean }>;
  executorProfiles: Array<{ id: string; name: string }>;
}) {
  const { t } = useTranslation();
  return (
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
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          <Label>{t("github:executorProfile")}</Label>
          <HelpTip
            text={t("github:theRepositoryWillBeAutomaticallyCloned", {
              path: "~/.kandev/repos/<owner>/<repo>",
            })}
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("github:optionalFallsBackToStepDefault2")}
        </p>
        <Select
          value={form.executorProfileId || STEP_DEFAULT}
          onValueChange={(v) =>
            setForm((prev) => ({ ...prev, executorProfileId: resolveProfileId(v) }))
          }
        >
          <SelectTrigger className="cursor-pointer">
            <SelectValue placeholder={STEP_DEFAULT_LABEL} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={STEP_DEFAULT}>{STEP_DEFAULT_LABEL}</SelectItem>
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
        <Label>{t("github:pollIntervalSeconds")}</Label>
        <p className="text-xs text-muted-foreground">{t("github:howOftenToCheckForNew2")}</p>
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
        description={cleanupPolicyDescription(t, REVIEW_CLEANUP_POLICY_OPTIONS, form.cleanupPolicy)}
        value={form.cleanupPolicy}
        onChange={(v) => setForm((prev) => ({ ...prev, cleanupPolicy: v as CleanupPolicy }))}
        placeholder={t("github:auto")}
        items={cleanupPolicyItems(t, REVIEW_CLEANUP_POLICY_OPTIONS)}
      />
    </>
  );
}

// Plain function, so `t` is threaded in — `mode: "jsx-only"` never inspects a
// function's return value, which is why these three survived the sweep.
function getSaveButtonLabel(t: TFunction, saving: boolean, isEditing: boolean): string {
  if (saving) return t("github:saving");
  return isEditing ? t("github:update") : t("github:create");
}

export function ReviewWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: ReviewWatchDialogProps) {
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
    form.customQuery.trim().length > 0 &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    form.prompt.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[900px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {watch ? t("github:editReviewWatch") : t("github:createReviewWatch")}
          </DialogTitle>
          <DialogDescription>{t("github:automaticallyCreateTasksWhenNewPull")}</DialogDescription>
        </DialogHeader>
        <WatchFormFields
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
            {getSaveButtonLabel(t, saving, !!watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
