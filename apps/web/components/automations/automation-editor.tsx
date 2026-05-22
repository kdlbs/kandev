"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Separator } from "@kandev/ui/separator";
import { IconArrowLeft, IconTrash } from "@tabler/icons-react";
import { useAutomations } from "@/hooks/domains/settings/use-automations";
import {
  addTrigger,
  updateTrigger,
  deleteTrigger,
  getAutomation,
  listTriggerTypes,
} from "@/lib/api/domains/automation-api";
import type {
  Automation,
  TriggerType,
  AutomationTrigger,
  TriggerTypeInfo,
  PlaceholderInfo,
  ExecutionMode,
} from "@/lib/types/automation";
import { TriggersSection } from "./triggers-section";
import { PromptSection } from "./prompt-section";
import { ConfigSection, type RepositorySelection } from "./config-section";
import { RunsSection } from "./runs-section";
import { createRepositoryAction } from "@/app/actions/workspaces";
import { generateUUID } from "@/lib/utils";

type AutomationEditorProps = {
  workspaceId: string;
  automationId: string | null; // null = create mode
};

type FormState = {
  name: string;
  description: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  // repositorySelection captures either a registered workspace repo (id),
  // a discovered local repo (path — registered at save time to obtain an
  // id), or "auto" for workspace-first fallback.
  repositorySelection: RepositorySelection;
  prompt: string;
  taskTitleTemplate: string;
  executionMode: ExecutionMode;
  enabled: boolean;
  maxConcurrentRuns: number;
};

type PendingTrigger = {
  tempId: string;
  type: TriggerType;
  config: Record<string, unknown>;
  enabled: boolean;
};

const DEFAULT_PROMPT = "Run scheduled automation.\n\nTrigger: {{trigger.type}}";

const defaultForm: FormState = {
  name: "",
  description: "",
  workflowId: "",
  workflowStepId: "",
  agentProfileId: "",
  executorProfileId: "",
  repositorySelection: { kind: "auto" },
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
      : { kind: "auto" },
    prompt: a.prompt || DEFAULT_PROMPT,
    taskTitleTemplate: a.task_title_template ?? "",
    executionMode: a.execution_mode ?? "task",
    enabled: a.enabled,
    maxConcurrentRuns: a.max_concurrent_runs,
  };
}

function pendingToTrigger(p: PendingTrigger): AutomationTrigger {
  return {
    id: p.tempId,
    automation_id: "",
    type: p.type,
    config: p.config,
    enabled: p.enabled,
    last_evaluated_at: null,
    created_at: "",
    updated_at: "",
  };
}

function useTriggerActions(currentId: string | null) {
  const [triggers, setTriggers] = useState<AutomationTrigger[]>([]);
  const [pending, setPending] = useState<PendingTrigger[]>([]);

  const handleAdd = async (type: TriggerType, config: Record<string, unknown>) => {
    if (!currentId) {
      setPending((prev) => [...prev, { tempId: generateUUID(), type, config, enabled: true }]);
      return;
    }
    const trigger = await addTrigger({ automation_id: currentId, type, config, enabled: true });
    setTriggers((prev) => [...prev, trigger]);
  };

  const handleUpdate = async (triggerId: string, config: Record<string, unknown>) => {
    if (!currentId) {
      setPending((prev) => prev.map((t) => (t.tempId === triggerId ? { ...t, config } : t)));
      return;
    }
    await updateTrigger(triggerId, { config });
    setTriggers((prev) => prev.map((t) => (t.id === triggerId ? { ...t, config } : t)));
  };

  const handleToggle = async (triggerId: string, enabled: boolean) => {
    if (!currentId) {
      setPending((prev) => prev.map((t) => (t.tempId === triggerId ? { ...t, enabled } : t)));
      return;
    }
    await updateTrigger(triggerId, { enabled });
    setTriggers((prev) => prev.map((t) => (t.id === triggerId ? { ...t, enabled } : t)));
  };

  const handleDelete = async (triggerId: string) => {
    if (!currentId) {
      setPending((prev) => prev.filter((t) => t.tempId !== triggerId));
      return;
    }
    await deleteTrigger(triggerId);
    setTriggers((prev) => prev.filter((t) => t.id !== triggerId));
  };

  const allTriggers = currentId ? triggers : pending.map(pendingToTrigger);
  const clearPending = () => setPending([]);

  return {
    triggers,
    setTriggers,
    pending,
    clearPending,
    allTriggers,
    handleAdd,
    handleUpdate,
    handleToggle,
    handleDelete,
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

// resolveRepositoryId turns a RepositorySelection into a concrete
// repository_id, registering a discovered local repo with the workspace
// first when needed. Empty string for "auto" (workspace-first fallback).
async function resolveRepositoryId(
  workspaceId: string,
  selection: RepositorySelection,
): Promise<string> {
  if (selection.kind === "auto") return "";
  if (selection.kind === "registered") return selection.id;
  const created = await createRepositoryAction({
    workspace_id: workspaceId,
    name: selection.name,
    source_type: "local",
    local_path: selection.path,
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: selection.defaultBranch,
    worktree_branch_prefix: "feature/",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
  });
  return created.id;
}

function buildCreatePayload(
  workspaceId: string,
  form: FormState,
  repositoryId: string,
  pending: PendingTrigger[],
) {
  return {
    workspace_id: workspaceId,
    name: form.name || "New Automation",
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    repository_id: repositoryId,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    execution_mode: form.executionMode,
    max_concurrent_runs: form.maxConcurrentRuns,
    triggers: pending.map((t) => ({ type: t.type, config: t.config, enabled: t.enabled })),
  };
}

type SaveHandlerOpts = {
  isNew: boolean;
  workspaceId: string;
  form: FormState;
  currentId: string | null;
  create: (payload: ReturnType<typeof buildCreatePayload>) => Promise<Automation>;
  update: (id: string, payload: ReturnType<typeof buildUpdatePayload>) => Promise<unknown>;
  setSaving: React.Dispatch<React.SetStateAction<boolean>>;
  setCurrentId: React.Dispatch<React.SetStateAction<string | null>>;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  triggerActions: ReturnType<typeof useTriggerActions>;
  router: ReturnType<typeof useRouter>;
};

// useSaveHandler returns the save callback for the automation editor.
// Pulled out of AutomationEditor to keep that component under the
// function-length lint cap; the save flow has gotten chunky now that it
// registers discovered repos before persisting the automation.
function useSaveHandler(opts: SaveHandlerOpts): () => Promise<void> {
  const { isNew, workspaceId, form, currentId, create, update } = opts;
  const { setSaving, setCurrentId, setForm, triggerActions, router } = opts;
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
        setCurrentId(a.id);
        triggerActions.setTriggers(a.triggers ?? []);
        triggerActions.clearPending();
        promoteSelection();
        router.replace(`/settings/workspace/${workspaceId}/automations/${a.id}`);
      } else if (currentId) {
        await update(currentId, buildUpdatePayload(form, repositoryId));
        promoteSelection();
      }
    } finally {
      setSaving(false);
    }
  };
}

function buildUpdatePayload(form: FormState, repositoryId: string) {
  return {
    name: form.name,
    description: form.description,
    workflow_id: form.workflowId,
    workflow_step_id: form.workflowStepId,
    agent_profile_id: form.agentProfileId,
    executor_profile_id: form.executorProfileId,
    repository_id: repositoryId,
    prompt: form.prompt,
    task_title_template: form.taskTitleTemplate,
    execution_mode: form.executionMode,
    enabled: form.enabled,
    max_concurrent_runs: form.maxConcurrentRuns,
  };
}

function getSaveLabel(saving: boolean, isNew: boolean): string {
  if (saving) return "Saving...";
  return isNew ? "Create Automation" : "Save Changes";
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

export function AutomationEditor({ workspaceId, automationId }: AutomationEditorProps) {
  const router = useRouter();
  const { create, update, remove } = useAutomations(workspaceId);
  const [form, setForm] = useState<FormState>(defaultForm);
  const [saving, setSaving] = useState(false);
  const [currentId, setCurrentId] = useState<string | null>(automationId);
  const isNew = currentId === null;
  const triggerActions = useTriggerActions(currentId);
  const triggerTypes = useTriggerTypeMetadata();

  const { placeholders, defaultTaskTitle, activeTriggerInfo, conditionType } = useConditionMetadata(
    triggerActions.allTriggers,
    triggerTypes,
  );
  useAutoPromptUpdate(activeTriggerInfo, conditionType, triggerTypes, setForm);

  useEffect(() => {
    if (!automationId) return;
    getAutomation(automationId)
      .then((a) => {
        setForm(formFromAutomation(a));
        triggerActions.setTriggers(a.triggers ?? []);
      })
      .catch(() => {
        router.push(`/settings/workspace/${workspaceId}/automations`);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);

  const updateField = useCallback(<K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  }, []);

  const handleSave = useSaveHandler({
    isNew,
    workspaceId,
    form,
    currentId,
    create,
    update,
    setSaving,
    setCurrentId,
    setForm,
    triggerActions,
    router,
  });

  const handleRemove = async () => {
    if (!currentId) return;
    await remove(currentId);
    router.push(`/settings/workspace/${workspaceId}/automations`);
  };

  const canSave = form.name.trim().length > 0 && !!form.workflowId && !!form.workflowStepId;

  return (
    <div className="max-w-3xl space-y-6" data-testid="automation-editor">
      <EditorHeader workspaceId={workspaceId} />
      <div className="space-y-2">
        <Label htmlFor="automation-name">Name</Label>
        <Input
          id="automation-name"
          data-testid="automation-name-input"
          value={form.name}
          onChange={(e) => updateField("name", e.target.value)}
          placeholder="Automation name"
        />
      </div>
      <Separator />
      <WhenSection
        triggerActions={triggerActions}
        triggerTypes={triggerTypes}
        currentId={currentId}
      />
      <Separator />
      <ThenSection
        form={form}
        workspaceId={workspaceId}
        placeholders={placeholders}
        defaultTaskTitle={defaultTaskTitle}
        conditionType={conditionType}
        updateField={updateField}
      />
      <Separator />
      <SettingsSection form={form} updateField={updateField} />
      <Separator />
      <RunsSection automationId={currentId} />
      <EditorFooter
        canSave={canSave}
        saving={saving}
        isNew={isNew}
        onSave={handleSave}
        onDelete={handleRemove}
      />
    </div>
  );
}

type TriggerActionsResult = ReturnType<typeof useTriggerActions>;

function WhenSection({
  triggerActions,
  triggerTypes,
  currentId,
}: {
  triggerActions: TriggerActionsResult;
  triggerTypes: TriggerTypeInfo[];
  currentId: string | null;
}) {
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">When</h3>
        <p className="text-sm text-muted-foreground">What causes this automation to run</p>
      </div>
      <div className="rounded-lg border bg-card p-4">
        <TriggersSection
          triggers={triggerActions.allTriggers}
          automationId={currentId}
          triggerTypes={triggerTypes}
          onAddTrigger={triggerActions.handleAdd}
          onUpdateTrigger={triggerActions.handleUpdate}
          onToggleTrigger={triggerActions.handleToggle}
          onDeleteTrigger={triggerActions.handleDelete}
        />
      </div>
    </div>
  );
}

function ThenSection({
  form,
  workspaceId,
  placeholders,
  defaultTaskTitle,
  conditionType,
  updateField,
}: {
  form: FormState;
  workspaceId: string;
  placeholders: PlaceholderInfo[];
  defaultTaskTitle: string;
  conditionType: TriggerType | null;
  updateField: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}) {
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">Then</h3>
        <p className="text-sm text-muted-foreground">
          A new task will be created each time this automation triggers
        </p>
      </div>
      <div className="rounded-lg border bg-card p-4 space-y-4">
        <div className="space-y-1.5">
          <Label className="text-xs">Task title</Label>
          <Input
            value={form.taskTitleTemplate}
            onChange={(e) => updateField("taskTitleTemplate", e.target.value)}
            placeholder={defaultTaskTitle || "[Auto] automation name"}
          />
          <p className="text-xs text-muted-foreground">
            Leave empty to use the default. Supports placeholders.
          </p>
        </div>
        <PromptSection
          value={form.prompt}
          onChange={(v) => updateField("prompt", v)}
          placeholders={placeholders}
        />
        <Separator />
        <ConfigSection
          workspaceId={workspaceId}
          workflowId={form.workflowId}
          workflowStepId={form.workflowStepId}
          agentProfileId={form.agentProfileId}
          executorProfileId={form.executorProfileId}
          repositorySelection={form.repositorySelection}
          executionMode={form.executionMode}
          conditionType={conditionType}
          onWorkflowChange={(v) => {
            updateField("workflowId", v);
            updateField("workflowStepId", "");
          }}
          onStepChange={(v) => updateField("workflowStepId", v)}
          onAgentProfileChange={(v) => updateField("agentProfileId", v)}
          onExecutorProfileChange={(v) => updateField("executorProfileId", v)}
          onRepositoryChange={(v) => updateField("repositorySelection", v)}
          onExecutionModeChange={(v) => updateField("executionMode", v)}
        />
      </div>
    </div>
  );
}

function EditorHeader({ workspaceId }: { workspaceId: string }) {
  const router = useRouter();
  return (
    <div className="flex items-center gap-3">
      <Button
        variant="ghost"
        size="icon-sm"
        className="cursor-pointer"
        onClick={() => router.push(`/settings/workspace/${workspaceId}/automations`)}
      >
        <IconArrowLeft className="h-4 w-4" />
      </Button>
      <span className="text-sm text-muted-foreground">Automations</span>
    </div>
  );
}

function SettingsSection({
  form,
  updateField,
}: {
  form: FormState;
  updateField: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}) {
  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">Settings</Label>
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <Switch
            checked={form.enabled}
            onCheckedChange={(v) => updateField("enabled", v)}
            className="cursor-pointer"
          />
          <Label className="text-sm">Enabled</Label>
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-sm">Max concurrent runs</Label>
          <Input
            type="number"
            min={1}
            value={form.maxConcurrentRuns}
            onChange={(e) => updateField("maxConcurrentRuns", parseInt(e.target.value) || 1)}
            className="w-20"
          />
        </div>
      </div>
    </div>
  );
}

function EditorFooter({
  canSave,
  saving,
  isNew,
  onSave,
  onDelete,
}: {
  canSave: boolean;
  saving: boolean;
  isNew: boolean;
  onSave: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="flex items-center gap-3 pt-4">
      <Button
        data-testid="automation-save-button"
        className="cursor-pointer"
        onClick={onSave}
        disabled={!canSave || saving}
      >
        {getSaveLabel(saving, isNew)}
      </Button>
      {!isNew && (
        <Button
          data-testid="automation-delete-button"
          variant="destructive"
          className="cursor-pointer"
          onClick={onDelete}
        >
          <IconTrash className="h-4 w-4 mr-1" />
          Delete
        </Button>
      )}
    </div>
  );
}
