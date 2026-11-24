"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useWorkflowSteps } from "@/hooks/use-workflow-steps";
import type { ContinuationPolicy, PlaceholderInfo, PREndAction } from "@/lib/types/automation";
import type { FormState } from "./automation-payload";
import { PromptSection } from "./prompt-section";
import { SELECTED_CARD_CLASS_NAME, UNSELECTED_CARD_CLASS_NAME } from "./automation-card-styles";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

type SectionProps = {
  form: FormState;
  savedForm: FormState;
  updateField: UpdateField;
};

// Phone and other coarse pointers need 44 px menu choices.
const TOUCH_ITEM_CLASS_NAME = "[@media(pointer:coarse)]:min-h-11";

// The Select needs a non-empty value for "stay in current step"; it maps to
// the persisted empty step ID.
const STAY_IN_STEP = "__stay__";
const PR_END_ACTIONS: PREndAction[] = ["keep", "archive", "delete"];

/** Clears every per-PR option, used when another policy is chosen. */
function clearPRContinuation(updateField: UpdateField) {
  updateField("followUpPrompt", "");
  updateField("prContinuationStepId", "");
  updateField("prMergedAction", "keep");
  updateField("prClosedAction", "keep");
}

type PolicyCardProps = {
  value: ContinuationPolicy;
  selected: boolean;
  title: string;
  description: string;
};

function PolicyCard({ value, selected, title, description }: PolicyCardProps) {
  const id = `automation-continuation-${value.replace(/_/g, "-")}`;
  return (
    <Label
      htmlFor={id}
      className={`flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3 ${
        selected ? SELECTED_CARD_CLASS_NAME : UNSELECTED_CARD_CLASS_NAME
      }`}
    >
      <RadioGroupItem
        id={id}
        value={value}
        aria-describedby={`${id}-description`}
        className="mt-0.5"
      />
      <span className="min-w-0 space-y-1">
        <span className="block text-sm font-medium">{title}</span>
        <span
          id={`${id}-description`}
          className="block whitespace-normal break-words text-xs text-muted-foreground"
        >
          {description}
        </span>
      </span>
    </Label>
  );
}

export function ContinuationPolicySection({
  form,
  savedForm,
  updateField,
  hasPRTrigger,
}: SectionProps & { hasPRTrigger: boolean }) {
  const { t } = useTranslation();
  const continuationIsDirty = form.continuationPolicy !== savedForm.continuationPolicy;
  const policy = form.continuationPolicy;
  const offerPerPR =
    hasPRTrigger || policy === "reuse_per_pr" || savedForm.continuationPolicy === "reuse_per_pr";
  const newTaskDescriptionKey =
    form.taskMode === "normal_task"
      ? "automations:contextBetweenRunsNewTaskNormalDescription"
      : "automations:contextBetweenRunsNewTaskDescription";
  const continuationDescriptionId = "automation-continuation-description";

  const handlePolicyChange = (value: string) => {
    const next = value as ContinuationPolicy;
    updateField("continuationPolicy", next);
    if (next === "reuse_thread") updateField("maxConcurrentRuns", 1);
    if (next !== "reuse_per_pr") clearPRContinuation(updateField);
  };

  return (
    <div className="space-y-2 border-t pt-3">
      <div>
        <h3 id="automation-continuation-heading" className="text-sm font-medium">
          {t("automations:contextBetweenRunsTitle")}
        </h3>
        <p id={continuationDescriptionId} className="text-xs text-muted-foreground">
          {t("automations:contextBetweenRunsDescription")}
        </p>
      </div>
      <RadioGroup
        aria-labelledby="automation-continuation-heading"
        aria-describedby={continuationDescriptionId}
        value={policy}
        onValueChange={handlePolicyChange}
        data-settings-dirty={continuationIsDirty}
        className="gap-2"
      >
        <PolicyCard
          value="new_task"
          selected={policy === "new_task"}
          title={t("automations:contextBetweenRunsNewTask")}
          description={t(newTaskDescriptionKey)}
        />
        <PolicyCard
          value="reuse_thread"
          selected={policy === "reuse_thread"}
          title={t("automations:contextBetweenRunsReuseThread")}
          description={t("automations:contextBetweenRunsReuseThreadDescription")}
        />
        {offerPerPR && (
          <PolicyCard
            value="reuse_per_pr"
            selected={policy === "reuse_per_pr"}
            title={t("automations:contextBetweenRunsReusePerPR")}
            description={t("automations:contextBetweenRunsReusePerPRDescription")}
          />
        )}
      </RadioGroup>
      {policy === "reuse_thread" && (
        <p className="text-xs text-muted-foreground">
          {t("automations:contextBetweenRunsConcurrencyLock")}
        </p>
      )}
      {policy === "reuse_per_pr" && (
        <PRContinuationOptions form={form} savedForm={savedForm} updateField={updateField} />
      )}
    </div>
  );
}

function PRContinuationOptions({ form, savedForm, updateField }: SectionProps) {
  return (
    <div className="space-y-3 pt-1">
      {form.taskMode === "normal_task" && (
        <ContinuationStepSelect form={form} savedForm={savedForm} updateField={updateField} />
      )}
      <PREndActionSelect
        field="prMergedAction"
        labelKey="automations:prMergedActionLabel"
        testId="pr-merged-action"
        form={form}
        savedForm={savedForm}
        updateField={updateField}
      />
      <PREndActionSelect
        field="prClosedAction"
        labelKey="automations:prClosedActionLabel"
        testId="pr-closed-action"
        form={form}
        savedForm={savedForm}
        updateField={updateField}
      />
    </div>
  );
}

function ContinuationStepSelect({ form, savedForm, updateField }: SectionProps) {
  const { t } = useTranslation();
  const { steps, loading } = useWorkflowSteps(form.workflowId);
  const value = form.prContinuationStepId || STAY_IN_STEP;
  return (
    <div className="space-y-1.5">
      <Label htmlFor="pr-continuation-step" className="text-xs">
        {t("automations:prContinuationStepLabel")}
      </Label>
      <Select
        value={value}
        onValueChange={(next) =>
          updateField("prContinuationStepId", next === STAY_IN_STEP ? "" : next)
        }
        disabled={loading}
      >
        <SelectTrigger
          id="pr-continuation-step"
          data-testid="pr-continuation-step"
          className="min-h-11 w-full sm:min-h-9"
          data-settings-dirty={form.prContinuationStepId !== savedForm.prContinuationStepId}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem className={TOUCH_ITEM_CLASS_NAME} value={STAY_IN_STEP}>
            {t("automations:prContinuationStepStay")}
          </SelectItem>
          {steps.map((step) => (
            <SelectItem className={TOUCH_ITEM_CLASS_NAME} key={step.id} value={step.id}>
              {step.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("automations:prContinuationStepHelp")}</p>
    </div>
  );
}

type PREndActionSelectProps = SectionProps & {
  field: "prMergedAction" | "prClosedAction";
  labelKey: string;
  testId: string;
};

function PREndActionSelect({
  field,
  labelKey,
  testId,
  form,
  savedForm,
  updateField,
}: PREndActionSelectProps) {
  const { t } = useTranslation();
  const labels: Record<PREndAction, string> = {
    keep: t("automations:prEndActionKeep"),
    archive: t("automations:prEndActionArchive"),
    delete: t("automations:prEndActionDelete"),
  };
  return (
    <div className="space-y-1.5">
      <Label htmlFor={testId} className="text-xs">
        {t(labelKey)}
      </Label>
      <Select value={form[field]} onValueChange={(next) => updateField(field, next as PREndAction)}>
        <SelectTrigger
          id={testId}
          data-testid={testId}
          className="min-h-11 w-full sm:min-h-9"
          data-settings-dirty={form[field] !== savedForm[field]}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PR_END_ACTIONS.map((action) => (
            <SelectItem className={TOUCH_ITEM_CLASS_NAME} key={action} value={action}>
              {labels[action]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {form[field] === "delete" && (
        <p role="note" className="text-xs text-destructive">
          {t("automations:prEndActionDeleteWarning")}
        </p>
      )}
    </div>
  );
}

/** The follow-up prompt, shown under the main prompt for reuse_per_pr. */
export function FollowUpPromptField({
  form,
  savedForm,
  updateField,
  placeholders,
}: SectionProps & { placeholders: PlaceholderInfo[] }) {
  const { t } = useTranslation();
  if (form.continuationPolicy !== "reuse_per_pr" || form.taskMode === "managed_conversation") {
    return null;
  }
  return (
    <PromptSection
      value={form.followUpPrompt}
      isDirty={form.followUpPrompt !== savedForm.followUpPrompt}
      onChange={(value) => updateField("followUpPrompt", value)}
      placeholders={placeholders}
      label={t("automations:followUpPromptLabel")}
      description={t("automations:followUpPromptHelp")}
      testId="automation-follow-up-prompt-editor"
    />
  );
}
