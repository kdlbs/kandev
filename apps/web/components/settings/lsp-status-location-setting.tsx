import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useTranslation } from "react-i18next";
import type { LspStatusLocation } from "@/lib/types/http";

type LspStatusLocationSettingProps = {
  value: LspStatusLocation;
  baseline: LspStatusLocation;
  onChange: (value: LspStatusLocation) => void;
};

const OPTIONS = [
  {
    value: "toolbar",
    labelKey: "lsp:editorToolbar",
    descriptionKey: "lsp:editorToolbarDescription",
  },
  {
    value: "status_bar",
    labelKey: "lsp:applicationStatusBar",
    descriptionKey: "lsp:applicationStatusBarDescription",
  },
] as const satisfies ReadonlyArray<{
  value: LspStatusLocation;
  labelKey: string;
  descriptionKey: string;
}>;

export function LspStatusLocationSetting({
  value,
  baseline,
  onChange,
}: LspStatusLocationSettingProps) {
  const { t } = useTranslation();
  const isDirty = value !== baseline;
  return (
    <div className="space-y-3" data-settings-dirty={isDirty}>
      <div>
        <div className="text-sm font-medium text-foreground">{t("lsp:statusLocation")}</div>
        <div className="text-xs text-muted-foreground">{t("lsp:chooseStatusLocation")}</div>
      </div>
      <RadioGroup
        aria-label={t("lsp:statusLocationAria")}
        value={value}
        onValueChange={(next) => onChange(next as LspStatusLocation)}
        className="grid gap-3 sm:grid-cols-2"
      >
        {OPTIONS.map((option) => (
          <Label
            key={option.value}
            htmlFor={`lsp-status-location-${option.value}`}
            className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 hover:bg-muted/30"
            data-settings-dirty={isDirty && value === option.value}
          >
            <RadioGroupItem
              id={`lsp-status-location-${option.value}`}
              value={option.value}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-1">
              <span className="block text-sm font-medium">{t(option.labelKey)}</span>
              <span className="block text-xs leading-relaxed text-muted-foreground">
                {t(option.descriptionKey)}
              </span>
            </span>
          </Label>
        ))}
      </RadioGroup>
    </div>
  );
}
