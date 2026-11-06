"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { generateUUID } from "@/lib/utils";
import { SettingsRow } from "../settings-group";
import { settingsActionClassName, settingsControlClassName } from "../settings-control";
import { gradeLimitDrafts, type GradeLimitDraft } from "./session-capacity-grade-draft";
import type { useSessionCapacitySettings } from "./use-session-capacity-settings";

export function SessionCapacityGradeEditor({
  state,
}: {
  state: ReturnType<typeof useSessionCapacitySettings>;
}) {
  const { t } = useTranslation();
  const disabled = !state.isAdmin || state.isLocked || !state.enabledDraft || !state.gradeSupported;
  const all = (state.isLocked ? state.snapshot?.effective.policy : state.policyDraft) === "all";
  const rows = state.isLocked
    ? gradeLimitDrafts(state.snapshot?.effective.grade_limits)
    : state.gradesDraft;
  return (
    <section className="space-y-3">
      <SettingsRow
        controlId="session-capacity-all"
        label={t("system:sessionCapacityAllLabel")}
        description={t("system:sessionCapacityAllHelp")}
        touchTarget="switch"
        control={
          <Switch
            id="session-capacity-all"
            aria-label={t("system:sessionCapacityAllLabel")}
            checked={all}
            disabled={disabled}
            onCheckedChange={(checked) => state.setPolicyDraft(checked ? "all" : "automatic")}
          />
        }
      />
      {!state.gradeSupported && (
        <p className="text-xs text-muted-foreground">
          {t("system:sessionCapacityGradesUnavailable")}
        </p>
      )}
      {state.gradeSupported && (all || rows.length > 0) && (
        <div className="space-y-3">
          {rows.map((row) => (
            <GradeLimitRow key={row.id} row={row} state={state} disabled={disabled} />
          ))}
          <Button
            type="button"
            variant="outline"
            disabled={disabled || !all}
            className={settingsActionClassName("cursor-pointer")}
            onClick={() =>
              state.setGradesDraft((current) => [
                ...current,
                { id: generateUUID(), grade: "", maximum: "1" },
              ])
            }
          >
            {t("system:sessionCapacityGradeAdd")}
          </Button>
          {state.scopeInvalid && (
            <p role="alert" className="text-sm text-destructive">
              {t("system:sessionCapacityGradeValidation")}
            </p>
          )}
        </div>
      )}
    </section>
  );
}

function GradeLimitRow({
  row,
  state,
  disabled,
}: {
  row: GradeLimitDraft;
  state: ReturnType<typeof useSessionCapacitySettings>;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const edit = (id: string, field: "grade" | "maximum", value: string) =>
    state.setGradesDraft((current) =>
      current.map((row) => (row.id === id ? { ...row, [field]: value } : row)),
    );
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-2 md:flex">
      <label className="min-w-0 flex-1 text-xs">
        <span>{t("system:sessionCapacityGradeName")}</span>
        <Input
          value={row.grade}
          disabled={disabled}
          onChange={(event) => edit(row.id, "grade", event.target.value)}
          className={settingsControlClassName("w-full")}
        />
      </label>
      <label className="min-w-0 text-xs md:w-28">
        <span>
          {t("system:sessionCapacityGradeMaximum", {
            grade: row.grade || t("system:sessionCapacityGradeNew"),
          })}
        </span>
        <Input
          type="number"
          inputMode="numeric"
          min={0}
          max={2147483647}
          step={1}
          value={row.maximum}
          disabled={disabled}
          onChange={(event) => edit(row.id, "maximum", event.target.value)}
          className={settingsControlClassName("w-full")}
        />
      </label>
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        className={settingsActionClassName("col-span-2 w-full cursor-pointer md:w-auto")}
        onClick={() =>
          state.setGradesDraft((current) => current.filter((value) => value.id !== row.id))
        }
      >
        {t("system:sessionCapacityGradeRemove", {
          grade: row.grade || t("system:sessionCapacityGradeNew"),
        })}
      </Button>
    </div>
  );
}
