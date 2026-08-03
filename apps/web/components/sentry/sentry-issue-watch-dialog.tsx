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
import { listSentryInstances, listSentryOrganizations } from "@/lib/api/domains/sentry-api";
import { WatcherRepositoryFields } from "@/components/watcher-repository-fields";
import { clearWorkspaceScopedForm } from "@/lib/watcher-repository-default";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";
import { sentryIssueWatchPlaceholders } from "./sentry-issue-watch-placeholders";
import { MaxInflightTasksField } from "./sentry-issue-watch-throttle-field";
import { SelectField, FilterFields, type FormSetter } from "./sentry-issue-watch-filter-fields";
import {
  type FormState,
  parseMaxInflightTasks,
  isWatchFormReady,
  buildWatchPayload,
  formStateFromWatch,
  makeEmptyForm,
} from "./sentry-issue-watch-form";
import type {
  CreateSentryIssueWatchRequest,
  SentryConfig,
  SentryIssueWatch,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

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

// `placeholders` comes from PromptField rather than being rebuilt here: this
// renders inside PromptField, which already needs the same array for
// ScriptEditor, so recomputing it would run the whole table twice per locale
// change for no benefit.
function PlaceholdersHelp({ placeholders }: { placeholders: ScriptPlaceholder[] }) {
  const { t } = useTranslation();
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">{t("sentry:availablePlaceholders")}</p>
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

function PromptField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  // Memoized because ScriptEditor keys its completion-provider registration on
  // `placeholders` identity. This used to be a module-scope const, so it was
  // stable for free; now that it is built from `t`, a fresh array on every
  // render would re-register the provider on every keystroke. The tooltip below
  // shares this array rather than building its own.
  const placeholders = useMemo(() => sentryIssueWatchPlaceholders(t), [t]);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>{t("sentry:taskPrompt")}</Label>
        <PlaceholdersHelp placeholders={placeholders} />
      </div>
      <p className="text-xs text-muted-foreground">
        {/* The `{{` token is passed as a value so it never reaches the catalog,
            where i18next would interpolate it away. */}
        {t("sentry:promptFieldHelp", { token: "{{" })}
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
      description={t("sentry:workspaceHelp")}
      value={value}
      onChange={onChange}
      placeholder={t("sentry:selectWorkspace")}
      items={workspaces.map((w) => ({ id: w.id, label: w.name }))}
      disabled={disabled}
    />
  );
}

// useWorkspaceInstances loads the workspace's Sentry instances for the required
// instance selector and auto-selects the sole instance on a fresh create.
function useWorkspaceInstances(
  open: boolean,
  workspaceId: string,
  hasWatch: boolean,
  setForm: FormSetter,
) {
  const [instances, setInstances] = useState<SentryConfig[]>([]);
  useEffect(() => {
    if (!open || !workspaceId) {
      setInstances([]);
      return;
    }
    let cancelled = false;
    listSentryInstances(workspaceId)
      .then((list) => {
        if (!cancelled) setInstances(list);
      })
      .catch(() => {
        if (!cancelled) setInstances([]);
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId]);
  useEffect(() => {
    if (hasWatch || instances.length !== 1) return;
    setForm((p) => (p.sentryInstanceId ? p : { ...p, sentryInstanceId: instances[0].id }));
  }, [hasWatch, instances, setForm]);
  return instances;
}

function InstancePicker({
  instances,
  value,
  onChange,
  disabled,
}: {
  instances: SentryConfig[];
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const noInstances = instances.length === 0;
  return (
    <SelectField
      label={t("sentry:sentryInstance")}
      description={t("sentry:sentryInstanceHelp")}
      value={value}
      onChange={onChange}
      placeholder={noInstances ? t("sentry:noInstancesInWorkspace") : t("sentry:selectAnInstance")}
      items={instances.map((i) => ({ id: i.id, label: i.name }))}
      disabled={disabled || noInstances}
    />
  );
}

function AutomationFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  const { workflows, agentProfiles, allExecutorProfiles } = useFormData(form.workspaceId);
  const { steps, loading: stepsLoading } = useWorkflowSteps(form.workflowId);
  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("sentry:workflow")}
          description={t("sentry:workflowHelp")}
          value={form.workflowId}
          onChange={(v) => setForm((p) => ({ ...p, workflowId: v, workflowStepId: "" }))}
          placeholder={t("sentry:selectWorkflow")}
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label={t("sentry:workflowStep")}
          description={t("sentry:workflowStepHelp")}
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
          label={t("sentry:agentProfile")}
          description={t("sentry:fallsBackToStepDefault")}
          value={form.agentProfileId}
          onChange={(v) => setForm((p) => ({ ...p, agentProfileId: v }))}
          placeholder={t("sentry:useStepDefault")}
          items={agentProfiles.map((p) => ({ id: p.id, label: p.label }))}
        />
        <SelectField
          label={t("sentry:executorProfile")}
          description={t("sentry:fallsBackToStepDefault")}
          value={form.executorProfileId}
          onChange={(v) => setForm((p) => ({ ...p, executorProfileId: v }))}
          placeholder={t("sentry:useStepDefault")}
          items={allExecutorProfiles.map((p) => ({ id: p.id, label: p.name }))}
        />
      </div>
    </>
  );
}

function SettingsFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-1.5">
        <Label>{t("sentry:pollIntervalSeconds")}</Label>
        <p className="text-xs text-muted-foreground">{t("sentry:pollIntervalHelp")}</p>
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
          <Label>{t("sentry:enabled")}</Label>
          <p className="text-xs text-muted-foreground">{t("sentry:enabledHelp")}</p>
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

// `t` is threaded in rather than read from a hook: this is a plain function, so
// a literal here would be invisible to the JSX-only guard.
function savingLabel(t: TFunction, saving: boolean, isEdit: boolean): string {
  if (saving) return t("sentry:saving");
  return isEdit ? t("sentry:update") : t("sentry:create");
}

// useWatchOrgs loads the org list for the org dropdown and auto-selects the
// sole org on a fresh create (with one choice there is nothing to pick).
function useWatchOrgs(
  open: boolean,
  workspaceId: string,
  instanceId: string,
  hasWatch: boolean,
  setForm: FormSetter,
) {
  const [orgs, setOrgs] = useState<string[]>([]);
  useEffect(() => {
    if (!open || !instanceId) {
      setOrgs([]);
      return;
    }
    setOrgs([]);
    let cancelled = false;
    listSentryOrganizations(workspaceId, instanceId)
      .then((res) => {
        if (!cancelled) setOrgs((res.organizations ?? []).map((o) => o.slug));
      })
      .catch(() => {
        if (!cancelled) setOrgs([]);
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, instanceId]);
  useEffect(() => {
    if (hasWatch || orgs.length !== 1) return;
    setForm((p) => (p.orgSlug ? p : { ...p, orgSlug: orgs[0] }));
  }, [hasWatch, orgs, setForm]);
  return orgs;
}

export function SentryIssueWatchDialog({
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

  useEffect(() => {
    if (watch) {
      setForm(formStateFromWatch(watch));
    } else {
      setForm(makeEmptyForm(workspaceId ?? activeWorkspaceId ?? ""));
    }
  }, [watch, open, workspaceId, activeWorkspaceId]);

  const instances = useWorkspaceInstances(open, form.workspaceId, !!watch, setForm);
  const orgs = useWatchOrgs(open, form.workspaceId, form.sentryInstanceId, !!watch, setForm);

  const workspaceLocked = true;

  const canSave = isWatchFormReady(form, { requiresInstance: !watch });

  const handleSave = useCallback(async () => {
    const maxInflight = parseMaxInflightTasks(form.maxInflightTasks);
    if (maxInflight === "invalid") return;
    setSaving(true);
    try {
      const payload = buildWatchPayload(form, maxInflight);
      if (watch) {
        await onUpdate(watch.id, watch.workspaceId, payload);
      } else {
        await onCreate({
          ...payload,
          workspaceId: form.workspaceId,
          sentryInstanceId: form.sentryInstanceId,
        });
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
            {watch ? t("sentry:editSentryWatcher") : t("sentry:createSentryWatcher")}
          </DialogTitle>
          <DialogDescription>{t("sentry:watchDialogDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <WorkspacePicker
            value={form.workspaceId}
            onChange={(v) => setForm((p) => clearWorkspaceScopedForm(p, v))}
            disabled={workspaceLocked}
          />
          <InstancePicker
            instances={instances}
            value={form.sentryInstanceId}
            onChange={(v) =>
              setForm((p) => ({ ...p, sentryInstanceId: v, orgSlug: "", projectSlugs: [] }))
            }
            disabled={!!watch}
          />
          <Separator />
          <FilterFields form={form} setForm={setForm} orgs={orgs} />
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
