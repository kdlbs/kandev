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
      <label
        className="flex min-h-7 cursor-pointer items-center justify-between gap-4 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        htmlFor="tool-payload-enabled"
      >
        <span>{t("system:toolPayload.enabled")}</span>
        <Switch
          id="tool-payload-enabled"
          data-testid="tool-payload-enabled"
          checked={draft.enabled}
          disabled={disabled}
          className="cursor-pointer"
          onCheckedChange={(enabled) => model.setEnabled(enabled)}
        />
      </label>
      <div className="space-y-2">
        <label htmlFor="tool-payload-age" className="text-sm font-medium">
          {t("system:toolPayload.age")}
        </label>
        <div className="grid min-w-0 grid-cols-1 gap-2 md:grid-cols-2">
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
            className={settingsControlClassName()}
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
              className={settingsControlClassName("w-full cursor-pointer")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="weeks">{t("system:toolPayload.weeks")}</SelectItem>
              <SelectItem value="months">{t("system:toolPayload.months")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <p id="tool-payload-age-help" className="text-xs text-muted-foreground">
          {t("system:toolPayload.ageHelp")}
        </p>
        {model.invalid && (
          <p role="alert" className="text-sm text-destructive">
            {t("system:toolPayload.invalidAge")}
          </p>
        )}
      </div>
    </>
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
