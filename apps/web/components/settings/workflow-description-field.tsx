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

// Always visible, unlike WorkflowPromptSection's collapsible — every synced workflow carries one.
export function WorkflowDescriptionField({
  workflow,
  savedWorkflow,
  readOnly,
  onUpdate,
}: WorkflowDescriptionFieldProps) {
  const { t } = useTranslation();
  const inputId = `workflow-description-input-${workflow.id}`;
  return (
    <div className="space-y-1.5">
      <Label htmlFor={inputId}>{t("workflows:workflowDescription")}</Label>
      <Textarea
        id={inputId}
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
