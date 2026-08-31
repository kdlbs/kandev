"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  normalizeWorkflowProfileSessionPolicy,
  type WorkflowProfileSessionPolicy,
  type Workflow,
} from "@/lib/types/http";
import { isWorkflowFieldDirty } from "./workflow-dirty-state";

const POLICY_OPTIONS: Array<{
  value: WorkflowProfileSessionPolicy;
  labelKey: string;
  descriptionKey: string;
}> = [
  {
    value: "complete",
    labelKey: "workflows:profileSessionPolicyComplete",
    descriptionKey: "workflows:profileSessionPolicyCompleteDescription",
  },
  {
    value: "park_reuse",
    labelKey: "workflows:profileSessionPolicyParkReuse",
    descriptionKey: "workflows:profileSessionPolicyParkReuseDescription",
  },
  {
    value: "park_new",
    labelKey: "workflows:profileSessionPolicyParkNew",
    descriptionKey: "workflows:profileSessionPolicyParkNewDescription",
  },
];

export function WorkflowProfileSessionPolicyField({
  workflow,
  savedWorkflow,
  onChange,
  readOnly,
}: {
  workflow: Workflow;
  savedWorkflow?: Workflow;
  onChange: (value: WorkflowProfileSessionPolicy) => void;
  readOnly: boolean;
}) {
  const { t } = useTranslation();
  const selectedValue = normalizeWorkflowProfileSessionPolicy(workflow.profile_session_policy);
  const selectedOption = POLICY_OPTIONS.find((option) => option.value === selectedValue)!;
  const isDirty = isWorkflowFieldDirty(workflow, savedWorkflow, "profile_session_policy");
  const controlId = `workflow-profile-session-policy-${workflow.id}`;

  return (
    <div className="w-full max-w-2xl space-y-1.5">
      <Label htmlFor={controlId}>{t("workflows:profileSessionPolicy")}</Label>
      <Select
        value={selectedValue}
        onValueChange={(next) => onChange(normalizeWorkflowProfileSessionPolicy(next))}
        disabled={readOnly}
      >
        <SelectTrigger
          id={controlId}
          className="min-h-11 w-full cursor-pointer"
          data-testid="workflow-profile-session-policy-select"
          data-settings-dirty={isDirty}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="max-w-[calc(100vw-2rem)]">
          {POLICY_OPTIONS.map((option) => (
            <SelectItem
              key={option.value}
              value={option.value}
              description={t(option.descriptionKey)}
              className="min-h-11"
            >
              {t(option.labelKey)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs leading-relaxed text-muted-foreground">
        {t(selectedOption.descriptionKey)}
      </p>
    </div>
  );
}
