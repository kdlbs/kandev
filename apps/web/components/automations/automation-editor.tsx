"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { toast } from "sonner";
import { Separator } from "@kandev/ui/separator";
import { useAutomations } from "@/hooks/domains/settings/use-automations";
import { getAutomation, listTriggerTypes } from "@/lib/api/domains/automation-api";
import type {
  Automation,
  CreateAutomationResponse,
  TriggerType,
  AutomationTrigger,
  TriggerTypeInfo,
} from "@/lib/types/automation";
import { RunsSection } from "./runs-section";
import {
  type CreatedWebhookDetails,
  type FormState,
  buildCreatePayload,
  buildUpdatePayload,
  buildWebhookUrl,
  resolveRepositoryId,
} from "./automation-payload";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
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

const DEFAULT_PROMPT = "Run scheduled automation.\n\nTrigger: {{trigger.type}}";

const defaultForm: FormState = {
  name: "",
  description: "",
  workflowId: "",
  workflowStepId: "",
  agentProfileId: "",
  executorProfileId: "",
  repositorySelection: { kind: "none" },
  prompt: DEFAULT_PROMPT,
  taskTitleTemplate: "",
  executionMode: "task",
  enabled: true,
  maxConcurrentRuns: 1,
};

function formFromAutomation(a: Automation): FormState {
  return {
    name: a.name,
    description: a.description,
    workflowId: a.workflow_id,
    workflowStepId: a.workflow_step_id,
    agentProfileId: a.agent_profile_id,
    executorProfileId: a.executor_profile_id,
    repositorySelection: a.repository_id
      ? { kind: "registered", id: a.repository_id }
      : { kind: "none" },
    prompt: a.prompt || DEFAULT_PROMPT,
    taskTitleTemplate: a.task_title_template ?? "",
    executionMode: a.execution_mode ?? "task",
    enabled: a.enabled,
    maxConcurrentRuns: a.max_concurrent_runs,
  };
}

function useTriggerTypeMetadata() {
  const [triggerTypes, setTriggerTypes] = useState<TriggerTypeInfo[]>([]);

  useEffect(() => {
    listTriggerTypes()
      .then(setTriggerTypes)
      .catch(() => setTriggerTypes([]));
  }, []);

  return triggerTypes;
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

type SaveHandlerOpts = {
  isNew: boolean;
  workspaceId: string;
  form: FormState;
  currentId: string | null;
  create: (payload: ReturnType<typeof buildCreatePayload>) => Promise<CreateAutomationResponse>;
  update: (id: string, payload: ReturnType<typeof buildUpdatePayload>) => Promise<unknown>;
  setSaving: React.Dispatch<React.SetStateAction<boolean>>;
  setCurrentId: React.Dispatch<React.SetStateAction<string | null>>;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  // setCreatedWebhook surfaces the URL + secret in a dialog after creating
  // a webhook automation, then the user is redirected to the listings page.
  // Null when no webhook trigger was configured on the new automation.
  setCreatedWebhook: React.Dispatch<React.SetStateAction<CreatedWebhookDetails | null>>;
  onSaved: (form: FormState, triggers: AutomationTrigger[]) => void;
  triggerActions: ReturnType<typeof useAutomationTriggerDrafts>;
  router: ReturnType<typeof useRouter>;
};

// useSaveHandler returns the save callback for the automation editor.
// Pulled out of AutomationEditor to keep that component under the
// function-length lint cap; the save flow has gotten chunky now that it
// registers discovered repos before persisting the automation.
function useSaveHandler(opts: SaveHandlerOpts): () => Promise<void> {
  const { isNew, workspaceId, form, currentId, create, update } = opts;
  const { setSaving, setCurrentId, setForm, setCreatedWebhook, triggerActions, router, onSaved } =
    opts;
  return async () => {
    setSaving(true);
    try {
      const repositoryId = await resolveRepositoryId(workspaceId, form.repositorySelection);
      const promoteSelection = () => {
        if (form.repositorySelection.kind === "discovered" && repositoryId) {
          setForm((prev) => ({
            ...prev,
            repositorySelection: { kind: "registered", id: repositoryId },
          }));
        }
      };
      if (isNew) {
        const a = await create(
          buildCreatePayload(workspaceId, form, repositoryId, triggerActions.pending),
        );
        promoteSelection();
        // Webhook automations need their URL + secret communicated to the
        // user; show the dialog and let its close handler do the redirect.
        // Everything else goes straight to the listings page with a toast.
        const hasWebhookTrigger = (a.triggers ?? []).some((t) => t.type === "webhook");
        if (hasWebhookTrigger && a.webhook_secret) {
          const savedForm =
            form.repositorySelection.kind === "discovered" && repositoryId
              ? {
                  ...form,
                  repositorySelection: { kind: "registered" as const, id: repositoryId },
                }
              : form;
          const savedTriggers = a.triggers ?? [];
          triggerActions.loadTriggers(savedTriggers);
          triggerActions.clearPending();
          onSaved(savedForm, savedTriggers);
          setCurrentId(a.id);
          setCreatedWebhook({ url: buildWebhookUrl(a.id), secret: a.webhook_secret });
        } else {
          toast.success("Automation created");
          runWithNavigationBlockerBypassed(() =>
            router.push(`/settings/workspace/${workspaceId}/automations`),
          );
        }
      } else if (currentId) {
        await update(currentId, buildUpdatePayload(form, repositoryId));
        const persistedTriggers = await triggerActions.persistDrafts();
        promoteSelection();
        const savedForm =
          form.repositorySelection.kind === "discovered" && repositoryId
            ? { ...form, repositorySelection: { kind: "registered" as const, id: repositoryId } }
            : form;
        onSaved(savedForm, persistedTriggers);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`Failed to save automation: ${msg}`);
      throw err;
    } finally {
      setSaving(false);
    }
  };
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
        setForm(loadedForm);
        loadTriggers(loadedTriggers);
        onLoaded(loadedForm, loadedTriggers);
      })
      .catch(() => {
        router.push(`/settings/workspace/${workspaceId}/automations`);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);
}

function useConditionMetadata(triggers: AutomationTrigger[], triggerTypes: TriggerTypeInfo[]) {
  const conditionType = getConditionType(triggers);
  const activeTriggerInfo = useMemo(
    () => triggerTypes.find((t) => t.type === (conditionType ?? "scheduled")),
    [triggerTypes, conditionType],
  );
  return {
    conditionType,
    placeholders: activeTriggerInfo?.placeholders ?? [],
    defaultTaskTitle: activeTriggerInfo?.default_task_title ?? "",
    activeTriggerInfo,
  };
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
  useSettingsSaveContributor({
    id: `automation:${currentId ?? "new"}`,
    revision,
    isDirty: isNew || revision !== savedRevision,
    canSave,
    invalidReason: canSave ? undefined : "Complete the required automation fields before saving.",
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
        router.push(`/settings/workspace/${workspaceId}/automations`),
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
      toast.error("Failed to delete automation", {
        description: error instanceof Error ? error.message : "Request failed",
      }),
  );
  const isRunMode = options.form.executionMode === "run";
  const canSave =
    options.form.name.trim().length > 0 &&
    (isRunMode || (!!options.form.workflowId && !!options.form.workflowStepId));
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
  const triggerTypes = useTriggerTypeMetadata();

  const { placeholders, defaultTaskTitle, activeTriggerInfo, conditionType } = useConditionMetadata(
    triggerActions.allTriggers,
    triggerTypes,
  );
  useAutoPromptUpdate(activeTriggerInfo, conditionType, triggerTypes, setForm);
  useLoadAutomation({
    automationId,
    workspaceId,
    setForm,
    loadTriggers: triggerActions.loadTriggers,
    onLoaded: (loadedForm) => setSavedForm(loadedForm),
    router,
  });

  const updateField = useCallback(<K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
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
    currentId,
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
    onSaved: (nextSavedForm) => setSavedForm(nextSavedForm),
  });
  const dirtyBaseline = isNew ? defaultForm : savedForm;
  const triggersDirty =
    triggerRevision(triggerActions.allTriggers) !==
    triggerRevision(triggerActions.baselineTriggers);

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
        conditionType={conditionType}
        savedForm={dirtyBaseline}
        updateField={updateField}
      />
      <Separator />
      <SettingsSection form={form} savedForm={dirtyBaseline} updateField={updateField} />
      <Separator />
      <RunsSection
        automationId={currentId}
        executionMode={form.executionMode}
        workspaceId={workspaceId}
      />
      <EditorFooter saving={saving} isNew={isNew} onDelete={handleRemove} />
      <CreatedWebhookDialogHost
        details={createdWebhook}
        onClose={() => {
          setCreatedWebhook(null);
          router.push(`/settings/workspace/${workspaceId}/automations`);
        }}
      />
    </div>
  );
}

function automationRevision(form: FormState, triggers: AutomationTrigger[]): string {
  return JSON.stringify({
    form,
    triggers: triggers.map(({ id, type, config, enabled }) => ({ id, type, config, enabled })),
  });
}

function triggerRevision(triggers: AutomationTrigger[]): string {
  return JSON.stringify(
    triggers.map(({ id, type, config, enabled }) => ({ id, type, config, enabled })),
  );
}
