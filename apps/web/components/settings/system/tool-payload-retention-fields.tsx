import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { settingsControlClassName } from "@/components/settings/settings-control";
import type { useToolPayloadRetentionDraft } from "@/hooks/domains/system/use-tool-payload-retention-draft";

type Draft = ReturnType<typeof useToolPayloadRetentionDraft>;
export function ToolPayloadRetentionFields({ model, pending }: { model: Draft; pending: boolean }) {
  const { t } = useTranslation();
  const { draft, setDraft, canEdit } = model;
  if (!draft) return null;
  const disabled = !canEdit || pending;
  return (
    <>
      <div className="flex flex-col gap-2 md:flex-row md:flex-wrap md:items-center">
        <label htmlFor="tool-payload-age" className="text-sm font-medium">
          {t("system:toolPayload.age")}
        </label>
        <div className="flex min-w-0 items-center gap-2">
          <Input
            id="tool-payload-age"
            data-testid="tool-payload-age"
            type="number"
            min={1}
            max={draft.age.unit === "weeks" ? 520 : 120}
            value={draft.age.value || ""}
            disabled={disabled}
            aria-invalid={model.invalid}
            aria-describedby="tool-payload-age-help"
            className={settingsControlClassName("w-20 shrink-0")}
            onChange={(e) =>
              setDraft({ ...draft, age: { ...draft.age, value: Number(e.target.value) } })
            }
          />
          <Select
            value={draft.age.unit}
            disabled={disabled}
            onValueChange={(unit) => {
              if (unit === "weeks" || unit === "months")
                setDraft({ ...draft, age: { ...draft.age, unit } });
            }}
          >
            <SelectTrigger
              aria-label={t("system:toolPayload.unit")}
              data-testid="tool-payload-unit"
              className={settingsControlClassName("w-32 cursor-pointer")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="weeks">{t("system:toolPayload.weeks")}</SelectItem>
              <SelectItem value="months">{t("system:toolPayload.months")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {model.invalid && (
          <p role="alert" className="text-sm text-destructive">
            {t("system:toolPayload.invalidAge")}
          </p>
        )}
      </div>
    </>
  );
}
export function ToolPayloadAutomation({ model, pending }: { model: Draft; pending: boolean }) {
  const { t } = useTranslation();
  const { draft, canEdit } = model;
  if (!draft) return null;
  const disabled = !canEdit || pending;
  return (
    <div className="space-y-1 border-t pt-4">
      <label
        className="flex min-h-7 cursor-pointer items-center gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        htmlFor="tool-payload-enabled"
      >
        <Switch
          id="tool-payload-enabled"
          data-testid="tool-payload-enabled"
          checked={draft.enabled}
          disabled={disabled}
          className="cursor-pointer"
          onCheckedChange={(enabled) => model.setEnabled(enabled)}
        />
        <span>{t("system:toolPayload.enabled")}</span>
      </label>
      <p className="text-xs text-muted-foreground">{t("system:toolPayload.schedule")}</p>
      <p className="text-xs text-muted-foreground">{t("system:toolPayload.firstRun")}</p>
    </div>
  );
}
export function ToolPayloadBackupReview({ model, pending }: { model: Draft; pending: boolean }) {
  const { t } = useTranslation();
  if (!model.needsChoice) return null;
  return (
    <fieldset disabled={!model.canEdit || pending} className="space-y-2 border-t pt-3">
      <legend className="text-sm font-medium">{t("system:toolPayload.beforeCleanup")}</legend>
      <p className="text-xs text-muted-foreground">{t("system:toolPayload.backupHelp")}</p>
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
              id={`tool-payload-${choice}`}
              data-testid={`tool-payload-${choice}`}
              value={choice}
              className="cursor-pointer"
            />
            <span className="text-sm">{t(`system:toolPayload.${choice}`)}</span>
          </label>
        ))}
      </RadioGroup>
    </fieldset>
  );
}
