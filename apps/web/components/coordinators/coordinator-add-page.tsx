"use client";

import { useState } from "react";
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

  const [form, setForm] = useState<CoordinatorFormState>(() => ({
    name: "",
    agentProfileId: resolveDefaultAgentProfileId(
      workspace?.default_agent_profile_id,
      agentProfiles,
    ),
    executorProfileId: resolveDefaultExecutorProfileId(workspace?.default_executor_id, executors),
    context: "",
  }));
  const [saving, setSaving] = useState(false);
  const [fieldError, setFieldError] = useState<CoordinatorFieldError | null>(null);

  const updateField = <K extends keyof CoordinatorFormState>(
    key: K,
    value: CoordinatorFormState[K],
  ) => {
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

  const canAdd = canAddCoordinator({
    name: form.name,
    agentProfileId: form.agentProfileId,
    executorProfileId: form.executorProfileId,
  });

  return (
    <div className="max-w-2xl space-y-6" data-testid="coordinator-add-page">
      <CoordinatorFormFields
        form={form}
        onChange={updateField}
        disabled={saving}
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
