"use client";

import { useTranslation } from "react-i18next";
import { useSandboxAccess } from "@/hooks/domains/settings/use-sandbox-access";
import { useSandboxSelectionModel } from "@/hooks/domains/settings/use-sandbox-selection-model";
import { SandboxUpdateError } from "@/lib/sandbox-messages";

export function useTaskEditSandbox(options: {
  open: boolean;
  task?: { id: string; workflowStepId: string } | null;
  hasStarted: boolean;
  agentProfileId?: string;
  executorId?: string;
}) {
  const { t } = useTranslation();
  const access = useSandboxAccess();
  const enabled = Boolean(access.enabled && options.open && options.task?.id);
  const model = useSandboxSelectionModel({
    scope: "task",
    targetId: options.task?.id ?? "",
    ...access,
    enabled,
    agentProfileId: options.hasStarted ? undefined : options.agentProfileId,
    executorId: options.executorId,
    workflowStepId: options.task?.workflowStepId,
  });
  const ready = !model.binding.dirty || model.canSave;
  return {
    blockedReason: ready ? undefined : t("agents:sandboxChooseEligible"),
    visible: enabled,
    model,
    isDirty: model.binding.dirty,
    ready,
    save: async () => {
      if (!model.canSave) throw new SandboxUpdateError(undefined);
      try {
        await model.binding.save();
      } catch (cause) {
        throw new SandboxUpdateError(cause);
      }
    },
  };
}
