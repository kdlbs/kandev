"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Label } from "@kandev/ui/label";
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
import { CliModeIcon } from "@/components/cli-mode-icon";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import { SettingsFields } from "./linear-issue-watch-fields";
import { FilterFields, SelectField } from "./linear-issue-watch-filter-fields";
import { linearIssueWatchPlaceholders } from "./linear-issue-watch-placeholders";
import { STEP_DEFAULT, STEP_DEFAULT_LABEL, resolveProfileId } from "@/lib/watcher-profile-default";
import { WatcherRepositoryFields } from "@/components/watcher-repository-fields";
import { clearWorkspaceScopedForm } from "@/lib/watcher-repository-default";
import {
  type FormState,
  buildWatchPayload,
  formStateFromWatch,
  isWatchFormReady,
  makeEmptyForm,
} from "./linear-issue-watch-form";
import type {
  CreateLinearIssueWatchInput,
  LinearIssueWatch,
  UpdateLinearIssueWatchInput,
} from "@/lib/types/linear";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

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

function PlaceholdersHelp() {
  const { t } = useTranslation();
  const placeholders = useMemo(() => linearIssueWatchPlaceholders(t), [t]);
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">{t("linear:availablePlaceholders")}</p>
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
  // Memoized because `ScriptEditor` keys its Monaco completion-provider
  // registration on `placeholders` identity. This used to be a module-scope
  // const, so it was stable for free; now that it is built from `t`, a fresh
  // array on every render would re-register the provider on every keystroke.
  const placeholders = useMemo(() => linearIssueWatchPlaceholders(t), [t]);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>{t("linear:taskPrompt")}</Label>
        <PlaceholdersHelp />
      </div>
      <p className="text-xs text-muted-foreground">
        {/* The `{{` token is passed as a value so it never reaches the catalog,
            where i18next would interpolate it away. */}
        {t("linear:promptFieldHelp", { token: "{{" })}
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
      description={t("linear:workspaceHelp")}
      value={value}
      onChange={onChange}
      placeholder={t("linear:selectWorkspace")}
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
          label={t("linear:workflow")}
          description={t("linear:workflowHelp")}
          value={form.workflowId}
          onChange={(v) => setForm((p) => ({ ...p, workflowId: v, workflowStepId: "" }))}
          placeholder={t("linear:selectWorkflow")}
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label={t("linear:workflowStep")}
          description={t("linear:workflowStepHelp")}
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
          label={t("linear:agentProfile")}
          description={t("linear:fallsBackToStepDefault")}
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
          label={t("linear:executorProfile")}
          description={t("linear:fallsBackToStepDefault")}
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

function savingLabel(t: TFunction, saving: boolean, isEdit: boolean): string {
  if (saving) return t("linear:saving");
  return isEdit ? t("linear:update") : t("linear:create");
}

export function LinearIssueWatchDialog({
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
          <DialogTitle>
            {watch ? t("linear:editLinearWatcher") : t("linear:createLinearWatcher")}
          </DialogTitle>
          <DialogDescription>{t("linear:watchDialogDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <WorkspacePicker
            value={form.workspaceId}
            onChange={(v) => setForm((p) => clearWorkspaceScopedForm(p, v))}
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
