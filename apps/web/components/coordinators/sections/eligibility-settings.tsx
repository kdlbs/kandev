"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { EligibilitySettings } from "@/lib/api/domains/coordinator-api";
import {
  ELIGIBILITY_BOUNDS,
  ELIGIBILITY_FIELDS,
  isEligibilityValueValid,
  type EligibilityField,
} from "@/lib/coordinators/control-draft";

const FIELD_LABEL: Record<EligibilityField, string> = {
  evidence_window_hours: "coordinator:eligibilityEvidenceWindow",
  min_decisions: "coordinator:eligibilityMinDecisions",
  unedited_floor_percent: "coordinator:eligibilityUneditedFloor",
  review_max_age_hours: "coordinator:eligibilityReviewMaxAge",
  daily_limit: "coordinator:eligibilityDailyLimit",
};

const FIELD_HELP: Record<EligibilityField, string> = {
  evidence_window_hours: "coordinator:eligibilityEvidenceWindowHelp",
  min_decisions: "coordinator:eligibilityMinDecisionsHelp",
  unedited_floor_percent: "coordinator:eligibilityUneditedFloorHelp",
  review_max_age_hours: "coordinator:eligibilityReviewMaxAgeHelp",
  daily_limit: "coordinator:eligibilityDailyLimitHelp",
};

type FieldProps = {
  field: EligibilityField;
  value: number;
  disabled: boolean;
  onChange: (value: number) => void;
};

function EligibilityFieldInput({ field, value, disabled, onChange }: FieldProps) {
  const { t } = useTranslation();
  const { min, max } = ELIGIBILITY_BOUNDS[field];
  const invalid = !isEligibilityValueValid(field, value);
  const id = `eligibility-${field}`;
  return (
    <div className="space-y-1" data-testid={`eligibility-field-${field}`}>
      <Label htmlFor={id}>{t(FIELD_LABEL[field])}</Label>
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        step={1}
        value={Number.isNaN(value) ? "" : value}
        disabled={disabled}
        aria-invalid={invalid}
        aria-describedby={`${id}-help`}
        className="max-w-40 max-md:h-11 [@media(pointer:coarse)]:h-11"
        onChange={(event) =>
          onChange(event.target.value === "" ? Number.NaN : Number(event.target.value))
        }
      />
      <p
        id={`${id}-help`}
        className={`text-xs ${invalid ? "text-destructive" : "text-muted-foreground"}`}
        data-testid={`eligibility-help-${field}`}
      >
        {t(FIELD_HELP[field])} {t("coordinator:eligibilityBounds", { min, max })}
      </p>
    </div>
  );
}

export type EligibilitySettingsGroupProps = {
  eligibility: EligibilitySettings;
  canManage: boolean;
  serverError?: string | null;
  onChange: (next: EligibilitySettings) => void;
};

/** The five bounded eligibility settings that gate raising a class to automatic. */
export function EligibilitySettingsGroup({
  eligibility,
  canManage,
  serverError,
  onChange,
}: EligibilitySettingsGroupProps) {
  const { t } = useTranslation();
  return (
    <section className="space-y-3 rounded-md border p-3" data-testid="eligibility-settings">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("coordinator:eligibilitySettingsTitle")}</h3>
        <p className="text-xs text-muted-foreground">
          {t("coordinator:eligibilitySettingsDescription")}
        </p>
      </div>
      {serverError && (
        <p role="alert" className="text-xs text-destructive" data-testid="eligibility-error">
          {serverError}
        </p>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        {ELIGIBILITY_FIELDS.map((field) => (
          <EligibilityFieldInput
            key={field}
            field={field}
            value={eligibility[field]}
            disabled={!canManage}
            onChange={(value) => onChange({ ...eligibility, [field]: value })}
          />
        ))}
      </div>
      <p className="text-xs text-muted-foreground" data-testid="eligibility-containment-note">
        {t("coordinator:eligibilityContainmentNote")}
      </p>
    </section>
  );
}
