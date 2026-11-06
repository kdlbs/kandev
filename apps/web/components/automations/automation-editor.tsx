"use client";

import { useTriggerTypeMetadata } from "@/hooks/use-automation-trigger-types";

import { findTriggerInfo } from "./plugin-condition";

import { useState, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { toast } from "@/lib/toast/sonner";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";
import { useAutomations } from "@/hooks/domains/settings/use-automations";
import { getAutomation } from "@/lib/api/domains/automation-api";
import type {
  Automation,
  TriggerType,
  AutomationTrigger,
  TriggerTypeInfo,
} from "@/lib/types/automation";
import { RunsSection } from "./runs-section";
import {
  type CreatedWebhookDetails,
  type FormState,
  DEFAULT_RETRY_POLICY,
  normalizeRetryPolicyForTaskMode,
  canonicalPluginTools,
  canonicalMCPSelection,
  automationDraftRevision,
} from "./automation-payload";
import {
  normalizeRepositorySelections,
  resolveExecutorType,
} from "./automation-repository-selection";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { useSaveHandler, type SaveHandlerOpts } from "./automation-editor-save";
import { AutomationDeleteConfirmDialog } from "./automation-delete-confirm-dialog";
import { useAutomationMCPReconciliation } from "@/hooks/domains/settings/use-automation-mcp-reconciliation";
import { AutomationMCPAccess } from "./automation-mcp-access";
import { AutomationPluginTools } from "./automation-plugin-tools";
import { useAutomationPluginTools } from "@/hooks/domains/settings/use-automation-plugin-tools";
import {
  CreatedWebhookDialogHost,
  EditorFooter,
  isAutomationFieldDirty,
  NameField,
  SettingsSection,
  ThenSection,
  WhenSection,
} from "./automation-editor-sections";

type AutomationEditorProps = {
  workspaceId: string;
  automationId: string | null; // null = create mode
};

// The seeded prompt is persisted on the automation and sent to the agent
// verbatim, and it is compared with `===` in useAutoPromptUpdate to decide
// whether the user has edited it. Both make it protocol, not copy — the same
// call the Jira (#2177), Linear (#2179) and Sentry (#2182) migrations made.
// i18n-exempt: persisted prompt, sent to the agent and compared with ===. See the comment above.
const DEFAULT_PROMPT = "Run scheduled automation.\n\nTrigger: {{trigger.type}}";

const defaultForm: FormState = {
  name: "",
  description: "",
  workflowId: "",
  workflowStepId: "",
  agentProfileId: "",
  executorProfileId: "",
  repositorySelections: [],
  prompt: DEFAULT_PROMPT,
  taskTitleTemplate: "",
  enabled: true,
  maxConcurrentRuns: 1,
  continuationPolicy: "new_task",
  taskMode: "automation_run",
  managedDestination: undefined,
  repositoryMode: "none",
  retryPolicy: DEFAULT_RETRY_POLICY,
  pluginTools: [],
};
/**
 * Initialize an editor draft from persisted settings, including the current MCP selection revision.
 */
function formFromAutomation(a: Automation): FormState {
  return {
    name: a.name,
    description: a.description,
    workflowId: a.workflow_id,
    workflowStepId: "",
    agentProfileId: a.agent_profile_id,
    executorProfileId: a.executor_profile_id,
    taskMode: a.task_mode ?? "automation_run",
    managedDestination: a.managed_destination,
    repositoryMode: (a.repositories?.length ?? a.repository_ids.length) > 0 ? "selected" : "none",
    repositorySelections: (
      a.repositories ?? a.repository_ids.map((id) => ({ repository_id: id, base_branch: "" }))
    ).map((repository, index) => ({
      kind: "registered" as const,
      id: repository.repository_id,
      branch: repository.base_branch,
      key: `automation-repository-${index}`,
    })),
    prompt: a.prompt || DEFAULT_PROMPT,
    taskTitleTemplate: a.task_title_template ?? "",
    enabled: a.enabled,
    maxConcurrentRuns: a.max_concurrent_runs,
    continuationPolicy: a.continuation_policy ?? "new_task",
    retryPolicy: a.retry_policy ?? DEFAULT_RETRY_POLICY,
    pluginTools: a.plugin_tools ?? [],
    mcpSelection: a.mcp_selection,
    mcpSelectionRevision: a.mcp_selection_revision ?? 0,
  };
}

/** Returns the condition type from the current triggers (the non-scheduled, non-webhook trigger). */
function getConditionType(triggers: AutomationTrigger[]): TriggerType | null {
  const condition = triggers.find((t) => t.type !== "scheduled" && t.type !== "webhook");
  return condition?.type ?? null;
}

/** Checks if the prompt matches any known default prompt from the trigger types. */
function isDefaultPrompt(prompt: string, triggerTypes: TriggerTypeInfo[]): boolean {
  return triggerTypes.some((t) => t.default_prompt === prompt);
}

/** Loads an existing automation on mount and populates form + trigger state. */
type LoadAutomationOpts = {
  automationId: string | null;
  workspaceId: string;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  loadTriggers: (triggers: AutomationTrigger[]) => void;
  onLoaded: (form: FormState, triggers: AutomationTrigger[]) => void;
  router: ReturnType<typeof useRouter>;
};

function useLoadAutomation(opts: LoadAutomationOpts) {
  const { automationId, workspaceId, setForm, loadTriggers, onLoaded, router } = opts;
  useEffect(() => {
    if (!automationId) return;
    getAutomation(automationId)
      .then((a) => {
        const loadedForm = formFromAutomation(a);
        const loadedTriggers = a.triggers ?? [];
        setForm({
          ...loadedForm,
          retryPolicy: normalizeRetryPolicyForTaskMode(loadedForm.taskMode, loadedForm.retryPolicy),
        });
        loadTriggers(loadedTriggers);
        onLoaded(loadedForm, loadedTriggers);
      })
      .catch(() => {
        router.push(`/settings/workspaces/${workspaceId}/automations`);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);
}

function useConditionMetadata(triggers: AutomationTrigger[], triggerTypes: TriggerTypeInfo[]) {
  const conditionType = getConditionType(triggers);
  const activeTriggerInfo = useMemo(
    () =>
      findTriggerInfo(
        triggers.find((trigger) => trigger.type === conditionType),
        triggerTypes,
      ),
    [triggerTypes, conditionType, triggers],
  );
  return {
    conditionType,
    placeholders: activeTriggerInfo?.placeholders ?? [],
    defaultTaskTitle: activeTriggerInfo?.default_task_title ?? "",
    activeTriggerInfo,
  };
}

// useSupportsMultiRepo mirrors ConfigSection's own capability check (see
// resolveExecutorType) so the save handler can enforce the same
// single-repository invariant the picker renders — see
// normalizeRepositorySelections for why this can't be skipped.
function useSupportsMultiRepo(executorProfileId: string): boolean {
  const executors = useAppStore((state) => state.executors.items);
  return (
    getMultiRepoExecutorDisabledReason(resolveExecutorType(executors, executorProfileId)) === null
  );
}

function useAutoPromptUpdate(
  activeTriggerInfo: TriggerTypeInfo | undefined,
  conditionType: TriggerType | null,
  triggerTypes: TriggerTypeInfo[],
  setForm: React.Dispatch<React.SetStateAction<FormState>>,
) {
  useEffect(() => {
    if (!activeTriggerInfo) return;
    setForm((prev) => {
      if (isDefaultPrompt(prev.prompt, triggerTypes) || prev.prompt === DEFAULT_PROMPT) {
        return { ...prev, prompt: activeTriggerInfo.default_prompt };
      }
      return prev;
    });
  }, [conditionType, activeTriggerInfo, triggerTypes, setForm]);
}

function useAutomationSaveContributor(options: {
  isNew: boolean;
  currentId: string | null;
  revision: string;
  savedRevision: string;
  canSave: boolean;
  save: () => Promise<void>;
  discard: () => void;
}) {
  const { isNew, currentId, revision, savedRevision, canSave, save, discard } = options;
  // `invalidReason` is resolved during RENDER, so it needs the hook rather than
  // the module-level `t` the toasts below use: nothing else in AutomationEditor
  // calls useTranslation(), so without this subscription the tooltip would keep
  // the previous locale's text until some unrelated re-render. (The toasts are
  // fine on the module-level `t` — they resolve at call time inside a callback.)
  const { t: translate } = useTranslation();
  useSettingsSaveContributor({
    id: `automation:${currentId ?? "new"}`,
    revision,
    isDirty: isNew || revision !== savedRevision,
    canSave,
    invalidReason: canSave ? undefined : translate("automations:completeRequiredFields"),
    save,
    discard,
  });
}

function useRemoveAutomation(
  currentId: string | null,
  workspaceId: string,
  remove: (id: string) => Promise<unknown>,
  router: ReturnType<typeof useRouter>,
  onError: (error: unknown) => void,
) {
  return async () => {
    if (!currentId) return;
    try {
      await remove(currentId);
      runWithNavigationBlockerBypassed(() =>
        router.push(`/settings/workspaces/${workspaceId}/automations`),
      );
    } catch (error) {
      onError(error);
      throw error;
    }
  };
}

type AutomationPersistenceOptions = SaveHandlerOpts & {
  savedRevision: string;
  discard: () => void;
  remove: (id: string) => Promise<unknown>;
};

function useAutomationPersistence(options: AutomationPersistenceOptions) {
  const handleSave = useSaveHandler(options);
  const handleRemove = useRemoveAutomation(
    options.currentId,
    options.workspaceId,
    options.remove,
    options.router,
    (error) =>
      toast.error(t("automations:failedToDeleteAutomation"), {
        // An Error message here is an API diagnostic and stays English; the
        // fallback for a missing payload is copy.
        description: error instanceof Error ? error.message : t("common:requestFailed"),
      }),
  );
  // The name is the only required field for hidden runs. A visible normal
  // task also needs a workflow so the task has a Kanban/sidebar destination.
  const canSave =
    options.form.name.trim().length > 0 &&
    (options.form.taskMode !== "normal_task" || options.form.workflowId.trim().length > 0) &&
    (options.form.taskMode !== "managed_conversation" ||
      Boolean(
        options.form.managedDestination?.plugin_id && options.form.managedDestination?.instance_key,
      )) &&
    (options.form.taskMode === "managed_conversation" ||
      options.form.repositorySelections.every(
        (selection) => selection.kind !== "none" && Boolean(selection.branch?.trim()),
      ));
  useAutomationSaveContributor({
    isNew: options.isNew,
    currentId: options.currentId,
    revision: automationRevision(options.form, options.triggerActions.allTriggers),
    savedRevision: options.savedRevision,
    canSave,
    save: handleSave,
    discard: options.discard,
  });
  return { handleRemove };
}

function useEditorDirtyState(
  isNew: boolean,
  savedForm: FormState,
  triggerActions: ReturnType<typeof useAutomationTriggerDrafts>,
) {
  const dirtyBaseline = isNew ? defaultForm : savedForm;
  const triggersDirty =
    triggerRevision(triggerActions.allTriggers) !==
    triggerRevision(triggerActions.baselineTriggers);
  return { dirtyBaseline, triggersDirty };
}

function AutomationDeleteControls({
  saving,
  isNew,
  automationName,
  onDelete,
}: {
  saving: boolean;
  isNew: boolean;
  automationName: string;
  onDelete: () => Promise<void>;
}) {
  const [confirmationOpen, setConfirmationOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleConfirm = useCallback(async () => {
    if (deleting) return;
    setDeleting(true);
    try {
      await onDelete();
    } catch {
      // onDelete already reports the deletion error to the user.
    } finally {
      setDeleting(false);
    }
  }, [deleting, onDelete]);

  return (
    <>
      <EditorFooter
        saving={saving || deleting}
        isNew={isNew}
        onDelete={() => setConfirmationOpen(true)}
      />
      <AutomationDeleteConfirmDialog
        open={confirmationOpen}
        automationName={automationName}
        isDeleting={deleting}
        onOpenChange={(open) => {
          if (!deleting) setConfirmationOpen(open);
        }}
        onConfirm={handleConfirm}
      />
    </>
  );
}

/**
 * Keep MCP choices and their saved revision with the automation draft throughout save and
 * conflict recovery.
 */
// eslint-disable-next-line max-lines-per-function -- The editor coordinates its form, trigger, settings-save, and deletion lifecycles.
export function AutomationEditor({ workspaceId, automationId }: AutomationEditorProps) {
  const router = useRouter();
  const { create, update, remove } = useAutomations(workspaceId);
  const [form, setForm] = useState<FormState>(defaultForm);
  const [saving, setSaving] = useState(false);
  const [currentId, setCurrentId] = useState<string | null>(automationId);
  const [createdWebhook, setCreatedWebhook] = useState<CreatedWebhookDetails | null>(null);
  const isNew = currentId === null;
  const triggerActions = useAutomationTriggerDrafts(currentId);
  const [savedForm, setSavedForm] = useState(defaultForm);
  const mcpReconciliation = useAutomationMCPReconciliation(
    workspaceId,
    currentId,
    setForm,
    setSavedForm,
  );
  const triggerTypes = useTriggerTypeMetadata(workspaceId);
  const pluginToolOptions = useAutomationPluginTools(
    workspaceId,
    form.taskMode === "automation_run",
  );

  const { placeholders, defaultTaskTitle, activeTriggerInfo, conditionType } = useConditionMetadata(
    triggerActions.allTriggers,
    triggerTypes,
  );
  useAutoPromptUpdate(activeTriggerInfo, conditionType, triggerTypes, setForm);
  const supportsMultiRepo = useSupportsMultiRepo(form.executorProfileId);
  useLoadAutomation({
    automationId,
    workspaceId,
    setForm,
    loadTriggers: triggerActions.loadTriggers,
    onLoaded: (loadedForm) => setSavedForm(loadedForm),
    router,
  });

  const updateField = useCallback(<K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((prev) => {
      const next = { ...prev, [key]: value };
      if (key === "taskMode") {
        return {
          ...next,
          retryPolicy: normalizeRetryPolicyForTaskMode(next.taskMode, prev.retryPolicy),
        };
      }
      return next;
    });
  }, []);

  const discard = useCallback(() => {
    setForm(isNew ? defaultForm : savedForm);
    triggerActions.discardDrafts();
  }, [isNew, savedForm, triggerActions]);
  const savedRevision = automationRevision(savedForm, triggerActions.baselineTriggers);
  const { handleRemove } = useAutomationPersistence({
    isNew,
    workspaceId,
    form,
    savedForm,
    currentId,
    supportsMultiRepo,
    create,
    update,
    setSaving,
    setCurrentId,
    setForm,
    setCreatedWebhook,
    triggerActions,
    router,
    savedRevision,
    discard,
    remove,
    onSaved: (nextSavedForm) => {
      setSavedForm(nextSavedForm);
      mcpReconciliation.saved();
    },
    onMCPSelectionSaveFailed: mcpReconciliation.failed,
    onPluginToolsSaveFailed: pluginToolOptions.reload,
  });
  const { dirtyBaseline, triggersDirty } = useEditorDirtyState(isNew, savedForm, triggerActions);

  return (
    <div className="max-w-3xl space-y-6" data-testid="automation-editor">
      <NameField
        value={form.name}
        isDirty={isAutomationFieldDirty(form, dirtyBaseline, "name")}
        onChange={(v) => updateField("name", v)}
      />
      <Separator />
      <WhenSection
        triggerActions={triggerActions}
        triggerTypes={triggerTypes}
        currentId={currentId}
        workspaceId={workspaceId}
        savedTriggers={triggerActions.baselineTriggers}
        isDirty={triggersDirty}
      />
      <Separator />
      <ThenSection
        form={form}
        workspaceId={workspaceId}
        placeholders={placeholders}
        defaultTaskTitle={defaultTaskTitle}
        savedForm={dirtyBaseline}
        updateField={updateField}
      />
      {form.taskMode !== "managed_conversation" ? (
        <AutomationMCPAccess
          saveFailed={mcpReconciliation.saveFailed}
          onReloadSaved={mcpReconciliation.reload}
          workspaceId={workspaceId}
          profileId={form.agentProfileId}
          repositoryIds={
            form.repositoryMode === "selected"
              ? normalizeRepositorySelections(form.repositorySelections, { supportsMultiRepo })
                  .filter((item) => item.kind === "registered")
                  .map((item) => item.id)
              : []
          }
          taskMode={form.taskMode}
          selection={form.mcpSelection}
          onChange={(value) => updateField("mcpSelection", value)}
          isDirty={
            JSON.stringify(canonicalMCPSelection(form.mcpSelection)) !==
            JSON.stringify(canonicalMCPSelection(dirtyBaseline.mcpSelection))
          }
        />
      ) : null}
      {form.taskMode === "automation_run" ? (
        <AutomationPluginTools
          groups={pluginToolOptions.groups}
          loading={pluginToolOptions.loading}
          error={pluginToolOptions.error}
          savedSelection={dirtyBaseline.pluginTools}
          selection={form.pluginTools}
          onChange={(value) => updateField("pluginTools", value)}
          onRetry={pluginToolOptions.reload}
          isDirty={
            JSON.stringify(canonicalPluginTools(form.pluginTools)) !==
            JSON.stringify(canonicalPluginTools(dirtyBaseline.pluginTools))
          }
        />
      ) : null}
      <Separator />
      <SettingsSection
        form={form}
        savedForm={dirtyBaseline}
        workspaceId={workspaceId}
        updateField={updateField}
      />
      <Separator />
      <RunsSection
        automationId={currentId}
        workspaceId={workspaceId}
        historyMode={form.retryPolicy.history_mode}
      />
      <AutomationDeleteControls
        saving={saving}
        isNew={isNew}
        automationName={savedForm.name}
        onDelete={handleRemove}
      />
      <CreatedWebhookDialogHost
        details={createdWebhook}
        onClose={() => {
          setCreatedWebhook(null);
          router.push(`/settings/workspaces/${workspaceId}/automations`);
        }}
      />
    </div>
  );
}

function automationRevision(form: FormState, triggers: AutomationTrigger[]): string {
  return JSON.stringify({
    form: automationDraftRevision(form),
    triggers: triggers.map(({ id, type, config, enabled }) => ({ id, type, config, enabled })),
  });
}

function triggerRevision(triggers: AutomationTrigger[]): string {
  return JSON.stringify(
    triggers.map(({ id, type, config, enabled }) => ({ id, type, config, enabled })),
  );
}
