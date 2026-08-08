"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { Workflow } from "@/lib/types/http";
import { isWorkflowFieldDirty } from "./workflow-dirty-state";

type WorkflowDescriptionFieldProps = {
  workflow: Workflow;
  savedWorkflow?: Workflow;
  readOnly?: boolean;
  onUpdate: (description: string) => void;
};

/**
 * Always visible (unlike WorkflowPromptSection's collapsible): every synced
 * workflow already carries a description by convention, so it belongs beside
 * Name/Agent Profile rather than tucked behind an optional disclosure.
 */
export function WorkflowDescriptionField({
  workflow,
  savedWorkflow,
  readOnly,
  onUpdate,
}: WorkflowDescriptionFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{t("workflows:workflowDescription")}</Label>
      <Textarea
        value={workflow.description ?? ""}
        onChange={(e) => onUpdate(e.target.value)}
        disabled={readOnly}
        placeholder={t("workflows:workflowDescriptionPlaceholder")}
        rows={2}
        className="min-h-16 text-sm"
        data-testid="workflow-description-input"
        data-settings-dirty={isWorkflowFieldDirty(workflow, savedWorkflow, "description")}
      />
    </div>
  );
}
