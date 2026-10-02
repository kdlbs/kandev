"use client";

import { useState } from "react";
import { useTaskPairPrefill } from "@/hooks/domains/coordinator/use-task-pair-prefill";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useCoordinators } from "@/hooks/domains/settings/use-coordinators";
import { toast } from "@/lib/toast/sonner";
import {
  buildCreateCoordinatorPayload,
  resolveDefaultAgentProfileId,
  resolveDefaultExecutorProfileId,
  type CoordinatorFormState,
} from "@/lib/coordinators/coordinator-form";
import { canAddCoordinator } from "@/lib/coordinators/validate-form";
import { coordinatorFieldError, type CoordinatorFieldError } from "@/lib/coordinators/field-error";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { CoordinatorFormFields } from "./coordinator-form-fields";
import type { WorkspaceState } from "@/lib/state/slices";

type Workspace = WorkspaceState["items"][number];

type CoordinatorAddPageProps = {
  workspaceId: string;
};

export function CoordinatorAddPage({ workspaceId }: CoordinatorAddPageProps) {
  const { t } = useTranslation();
  const router = useRouter();
  useSettingsData(true);
  const { create } = useCoordinators(workspaceId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null,
  );
  const canManage = hasScope(workspace?.scopes, SCOPE.workspaceManage);

  const [form, setForm] = useState<CoordinatorFormState>(() => ({
    name: "",
    agentProfileId: resolveDefaultAgentProfileId(
      workspace?.default_agent_profile_id,
      agentProfiles,
    ),
    executorProfileId: resolveDefaultExecutorProfileId(workspace?.default_executor_id, executors),
    taskAgentProfileId: "",
    taskExecutorProfileId: "",
    context: "",
  }));
  const [taskTouched, setTaskTouched] = useState({ agent: false, executor: false });
  const agentsLoaded = useAppStore((state) => state.settingsData.agentsLoaded);
  useTaskPairPrefill({
    workspaceDefaultAgentProfileId: workspace
      ? (workspace.default_agent_profile_id ?? null)
      : undefined,
    agentProfiles: agentsLoaded ? agentProfiles : undefined,
    ownAgent: form.agentProfileId,
    ownExecutor: form.executorProfileId,
    touched: taskTouched,
    current: { agent: form.taskAgentProfileId, executor: form.taskExecutorProfileId },
    onChange: (pair) =>
      setForm((prev) => ({
        ...prev,
        taskAgentProfileId: pair.agent,
        taskExecutorProfileId: pair.executor,
      })),
  });
  const [saving, setSaving] = useState(false);
  const [fieldError, setFieldError] = useState<CoordinatorFieldError | null>(null);

  const updateField = <K extends keyof CoordinatorFormState>(
    key: K,
    value: CoordinatorFormState[K],
  ) => {
    if (key === "taskAgentProfileId") setTaskTouched((prev) => ({ ...prev, agent: true }));
    if (key === "taskExecutorProfileId") setTaskTouched((prev) => ({ ...prev, executor: true }));
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const handleAdd = async () => {
    setSaving(true);
    setFieldError(null);
    try {
      const created = await create(buildCreateCoordinatorPayload(form));
      router.replace(`/settings/workspaces/${workspaceId}/coordinators/${created.id}`);
    } catch (error) {
      const nextFieldError = coordinatorFieldError(error);
      setFieldError(nextFieldError);
      if (!nextFieldError) {
        toast.error(t("coordinator:failedToAddCoordinator"));
      }
    } finally {
      setSaving(false);
    }
  };

  const canAdd =
    canManage &&
    canAddCoordinator({
      name: form.name,
      agentProfileId: form.agentProfileId,
      executorProfileId: form.executorProfileId,
      taskAgentProfileId: form.taskAgentProfileId,
      taskExecutorProfileId: form.taskExecutorProfileId,
    });

  return (
    <div className="max-w-2xl space-y-6" data-testid="coordinator-add-page">
      <CoordinatorFormFields
        form={form}
        onChange={updateField}
        disabled={!canManage || saving}
        agentProfiles={agentProfiles}
        executors={executors}
        fieldError={fieldError}
      />
      <Button
        type="button"
        data-testid="add-coordinator-submit"
        className="cursor-pointer"
        disabled={!canAdd || saving}
        onClick={handleAdd}
      >
        {t("coordinator:addCoordinator")}
      </Button>
    </div>
  );
}
