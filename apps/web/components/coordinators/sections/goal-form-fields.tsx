"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { generateUUID } from "@/lib/uuid";
import type { GoalFormState } from "@/lib/coordinators/goal-form";
import type { GoalFieldError } from "./use-goal-editor";

type GoalFormFieldsProps = {
  form: GoalFormState;
  disabled: boolean;
  fieldError: GoalFieldError | null;
  onChange: (next: GoalFormState) => void;
  onToggle?: (criterionId: string, done: boolean) => void;
  /** Draft mode: no stored criteria, so no done checkboxes. */
  draft?: boolean;
  /** One line under the criteria list. */
  criteriaMessage?: string | null;
  /** Draft mode: messages by `name`, `due_on` and `criteria[i].text`. */
  messages?: Record<string, string>;
};

function criterionTextField(index: number): string {
  return ["criteria[", index, "].text"].join("");
}

function FieldMessage({
  error,
  messages,
  field,
}: {
  error?: GoalFieldError | null;
  messages?: Record<string, string>;
  field: string;
}) {
  const message = messages?.[field] ?? (error?.field === field ? error.message : undefined);
  if (!message) return null;
  return (
    <p role="alert" className="text-xs text-destructive">
      {message}
    </p>
  );
}

type CriterionRowProps = {
  criterion: GoalFormState["criteria"][number];
  index: number;
  draft: boolean;
  disabled?: boolean;
  fieldError?: GoalFieldError | null;
  messages?: GoalFormFieldsProps["messages"];
  onText: (text: string) => void;
  onToggle?: GoalFormFieldsProps["onToggle"];
  onRemove: () => void;
};

function CriterionRow({
  criterion,
  index,
  draft,
  disabled,
  fieldError,
  messages,
  onText,
  onToggle,
  onRemove,
}: CriterionRowProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2" data-testid="goal-criterion">
      {!draft && (
        <Checkbox
          checked={criterion.done}
          disabled={disabled || !criterion.id}
          aria-label={t("coordinator:goalCriterionDoneLabel", { number: index + 1 })}
          onCheckedChange={(checked) => criterion.id && onToggle?.(criterion.id, checked === true)}
        />
      )}
      <Input
        value={criterion.text}
        disabled={disabled}
        aria-label={t("coordinator:goalCriterionLabel", { number: index + 1 })}
        onChange={(event) => onText(event.target.value)}
      />
      {!disabled && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="cursor-pointer"
          onClick={onRemove}
        >
          {t("coordinator:goalRemoveCriterion")}
        </Button>
      )}
      <FieldMessage error={fieldError} messages={messages} field={criterionTextField(index)} />
    </div>
  );
}

export function GoalFormFields({
  form,
  disabled,
  fieldError,
  onChange,
  onToggle,
  draft = false,
  criteriaMessage,
  messages,
}: GoalFormFieldsProps) {
  const { t } = useTranslation();
  const setCriterion = (key: string, text: string) =>
    onChange({
      ...form,
      criteria: form.criteria.map((c) => (c.key === key ? { ...c, text } : c)),
    });

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <Label htmlFor="goal-name">{t("coordinator:goalMilestoneLabel")}</Label>
        <Input
          id="goal-name"
          data-testid="goal-name"
          value={form.name}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, name: event.target.value })}
        />
        <FieldMessage error={fieldError} messages={messages} field="name" />
      </div>
      <div className="space-y-1">
        <Label htmlFor="goal-due">{t("coordinator:goalDueLabel")}</Label>
        <Input
          id="goal-due"
          data-testid="goal-due"
          type="date"
          value={form.dueOn}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, dueOn: event.target.value })}
        />
        <FieldMessage error={fieldError} messages={messages} field="due_on" />
      </div>
      <div className="space-y-2">
        <p className="text-sm font-medium">{t("coordinator:goalExitCriteria")}</p>
        {form.criteria.map((criterion, index) => (
          <CriterionRow
            key={criterion.key}
            criterion={criterion}
            index={index}
            draft={draft}
            disabled={disabled}
            fieldError={fieldError}
            messages={messages}
            onText={(text) => setCriterion(criterion.key, text)}
            onToggle={onToggle}
            onRemove={() =>
              onChange({ ...form, criteria: form.criteria.filter((c) => c.key !== criterion.key) })
            }
          />
        ))}
        {criteriaMessage && (
          <p role="alert" className="text-xs text-destructive" data-testid="goal-criteria-error">
            {criteriaMessage}
          </p>
        )}
        {!disabled && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            data-testid="goal-add-criterion"
            className="cursor-pointer"
            onClick={() =>
              onChange({
                ...form,
                criteria: [...form.criteria, { key: generateUUID(), text: "", done: false }],
              })
            }
          >
            {t("coordinator:goalAddCriterion")}
          </Button>
        )}
      </div>
    </div>
  );
}
