import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { settingsControlClassName } from "@/components/settings/settings-control";
import type { useArchivedDataRetentionDraft } from "@/hooks/domains/system/use-archived-data-retention-draft";
import {
  maxArchivedDays,
  validArchivedDays,
} from "@/hooks/domains/system/use-archived-data-retention-draft";
import type { ArchivedDataPolicy } from "@/lib/types/archived-data-retention";

type Draft = ReturnType<typeof useArchivedDataRetentionDraft>;
type AgeKey = "archived_age" | "cleanup_age";

const ageFields: { key: AgeKey; label: string; testId: string; helpId: string }[] = [
  {
    key: "archived_age",
    label: "system:archivedData.archivedAge",
    testId: "archived-data-archived-age",
    helpId: "archived-data-archived-age-help",
  },
  {
    key: "cleanup_age",
    label: "system:archivedData.cleanupAge",
    testId: "archived-data-cleanup-age",
    helpId: "archived-data-cleanup-age-help",
  },
];

export function ArchivedDataRetentionFields({
  model,
  pending,
}: {
  model: Draft;
  pending: boolean;
}) {
  const { t } = useTranslation();
  const { draft, setDraft, canEdit } = model;
  if (!draft) return null;
  const disabled = !canEdit || pending;
  const setAge = (key: AgeKey, days: number) =>
    setDraft({ ...draft, [key]: { days } } as ArchivedDataPolicy);
  return (
    <div className="space-y-2">
      {ageFields.map((field) => {
        const age = draft[field.key];
        return (
          <div key={field.key} className="flex flex-col gap-2 md:flex-row md:items-center">
            <label htmlFor={field.testId} className="text-sm font-medium md:w-64">
              {t(field.label)}
            </label>
            <Input
              id={field.testId}
              data-testid={field.testId}
              type="number"
              min={1}
              max={maxArchivedDays}
              value={age.days || ""}
              disabled={disabled}
              aria-invalid={!validArchivedDays(age)}
              aria-describedby={field.helpId}
              className={settingsControlClassName("w-24 shrink-0")}
              onChange={(e) => setAge(field.key, Number(e.target.value))}
            />
            <p id={field.helpId} className="text-xs text-muted-foreground">
              {t(
                field.key === "archived_age"
                  ? "system:archivedData.archivedAgeHelp"
                  : "system:archivedData.cleanupAgeHelp",
              )}
            </p>
          </div>
        );
      })}
    </div>
  );
}

export function ArchivedDataAutomation({ model, pending }: { model: Draft; pending: boolean }) {
  const { t } = useTranslation();
  const { draft, canEdit } = model;
  if (!draft) return null;
  const disabled = !canEdit || pending;
  return (
    <div className="space-y-1 border-t pt-4">
      <label
        className="flex min-h-7 cursor-pointer items-center gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        htmlFor="archived-data-enabled"
      >
        <Switch
          id="archived-data-enabled"
          data-testid="archived-data-enabled"
          checked={draft.enabled}
          disabled={disabled}
          className="cursor-pointer"
          onCheckedChange={(enabled) => model.setEnabled(enabled)}
        />
        <span>{t("system:archivedData.enabled")}</span>
      </label>
      <p className="text-xs text-muted-foreground">{t("system:archivedData.schedule")}</p>
      <p className="text-xs text-muted-foreground">{t("system:archivedData.firstRun")}</p>
      <p className="text-xs text-muted-foreground">{t("system:archivedData.compactionHelp")}</p>
    </div>
  );
}

export function ArchivedDataBackupReview({ model, pending }: { model: Draft; pending: boolean }) {
  const { t } = useTranslation();
  if (!model.needsChoice) return null;
  return (
    <fieldset disabled={!model.canEdit || pending} className="space-y-2 border-t pt-3">
      <legend className="text-sm font-medium">{t("system:archivedData.beforeCleanup")}</legend>
      <p className="text-xs text-muted-foreground">{t("system:archivedData.backupHelp")}</p>
      <RadioGroup
        value={model.choice}
        disabled={!model.canEdit || pending}
        onValueChange={(value) => {
          if (value === "backup" || value === "skip") model.setChoice(value);
        }}
      >
        {(["backup", "skip"] as const).map((choice) => (
          <label
            key={choice}
            className="flex min-h-7 cursor-pointer items-center gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          >
            <RadioGroupItem
              id={`archived-data-${choice}`}
              data-testid={`archived-data-${choice}`}
              value={choice}
              className="cursor-pointer"
            />
            <span className="text-sm">{t(`system:archivedData.${choice}`)}</span>
          </label>
        ))}
      </RadioGroup>
    </fieldset>
  );
}
