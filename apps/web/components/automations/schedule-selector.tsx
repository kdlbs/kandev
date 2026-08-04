"use client";

import { useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";

type ScheduleSelectorProps = {
  config: Record<string, unknown> | null;
  isDirty?: boolean;
  onChange: (config: Record<string, unknown>) => void;
};

const SHORTHANDS = new Set(["@hourly", "@daily", "@weekly", "0 * * * *", "0 0 * * *", "0 0 * * 0"]);
const EVERY_RE = /^@every\s+(\d+[hms])+$/;
const CRON_FIELD_RE = /^(\*|\*\/\d+|\d+)(\s+(\*|\*\/\d+|\d+)){4}$/;

function isValidExpression(expr: string): boolean {
  const trimmed = expr.trim();
  if (!trimmed) return true;
  if (SHORTHANDS.has(trimmed)) return true;
  if (EVERY_RE.test(trimmed)) return true;
  if (CRON_FIELD_RE.test(trimmed)) return true;
  return false;
}

// `expression` is the cron/shorthand syntax the backend parses and persists —
// never translated. The label is copy: the interval presets carry a `count` so
// the plural form is chosen by i18next rather than baked into English, and the
// two named presets are plain messages.
const PRESETS: ReadonlyArray<{ labelKey: string; count?: number; expression: string }> = [
  { labelKey: "automations:presetMinutes", count: 5, expression: "@every 5m" },
  { labelKey: "automations:presetMinutes", count: 15, expression: "@every 15m" },
  { labelKey: "automations:presetMinutes", count: 30, expression: "@every 30m" },
  { labelKey: "automations:presetHours", count: 1, expression: "@hourly" },
  { labelKey: "automations:presetHours", count: 6, expression: "@every 6h" },
  { labelKey: "automations:presetDaily", expression: "@daily" },
  { labelKey: "automations:presetWeekly", expression: "@weekly" },
];

// Cron syntax shown inside the help text. These travel as interpolation values
// so the pseudo-locale cannot accent them into an expression that no longer
// parses — see docs/i18n.md.
const CRON_EVERY = "@every";
const CRON_EVERY_10M = "@every 10m";
const CRON_EVERY_2H30M = "@every 2h30m";
const CRON_HOURLY = "@hourly";
const CRON_DAILY = "@daily";
const CRON_WEEKLY = "@weekly";
const codeClass = "bg-muted px-1 rounded";

export function ScheduleSelector({ config, isDirty = false, onChange }: ScheduleSelectorProps) {
  const { t } = useTranslation();
  const configExpr = (config?.cron_expression as string) ?? "";
  const [customInput, setCustomInput] = useState(configExpr);
  const [error, setError] = useState<string | null>(null);

  // Re-sync the input when the saved config arrives or changes from elsewhere
  // (e.g. async automation fetch on page reload). useState's initial value
  // only fires at mount, so without this the input would stay empty after
  // a deferred load.
  useEffect(() => {
    setCustomInput(configExpr);
  }, [configExpr]);

  const handlePreset = (expression: string) => {
    setCustomInput(expression);
    setError(null);
    onChange({ cron_expression: expression });
  };

  const handleCustomBlur = () => {
    const trimmed = customInput.trim();
    // An empty input means "clear the schedule" — propagate so the saved
    // config doesn't retain a stale cron expression.
    if (trimmed === "") {
      setError(null);
      if (configExpr !== "") onChange({ cron_expression: "" });
      return;
    }
    if (!isValidExpression(trimmed)) {
      // `@every` is the token the scheduler accepts and the user must type, so
      // it is interpolated rather than left in the catalog — otherwise the
      // pseudo-locale renders `@ēvēŕŷ` and the error tells them to type
      // something that can never parse.
      setError(t("automations:invalidExpression", { every: CRON_EVERY }));
      return;
    }
    setError(null);
    onChange({ cron_expression: trimmed });
  };

  return (
    <div className="space-y-2" data-testid="schedule-selector">
      <div className="flex items-center gap-1.5 flex-wrap">
        {PRESETS.map((preset) => (
          <Button
            key={preset.expression}
            data-testid={`schedule-preset-${preset.expression}`}
            variant={configExpr === preset.expression ? "secondary" : "outline"}
            size="sm"
            className="cursor-pointer"
            onClick={() => handlePreset(preset.expression)}
          >
            {t(preset.labelKey, { count: preset.count })}
          </Button>
        ))}
        <Tooltip>
          <TooltipTrigger asChild>
            <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground ml-1" />
          </TooltipTrigger>
          <TooltipContent className="max-w-[280px]">
            {t("automations:scheduleTooltip")}
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="space-y-1">
        <Label className="text-xs text-muted-foreground">
          {t("automations:customIntervalLabel")}
        </Label>
        <Input
          value={customInput}
          onChange={(e) => {
            setCustomInput(e.target.value);
            if (error) setError(null);
          }}
          onBlur={handleCustomBlur}
          data-testid="schedule-custom-input"
          data-settings-dirty={isDirty}
          // Cron syntax the user types verbatim, not copy — same value the
          // help text below interpolates.
          placeholder={CRON_EVERY_2H30M}
          className={`font-mono text-sm max-w-xs ${error ? "border-destructive" : ""}`}
        />
        {error && <p className="text-xs text-destructive">{error}</p>}
        <p className="text-xs text-muted-foreground">
          <Trans
            i18nKey="automations:scheduleSyntaxHelp"
            values={{
              every: CRON_EVERY,
              example1: CRON_EVERY_10M,
              example2: CRON_EVERY_2H30M,
              hourly: CRON_HOURLY,
              daily: CRON_DAILY,
              weekly: CRON_WEEKLY,
            }}
          >
            <code className={codeClass} />
            <code className={codeClass} />
            <code className={codeClass} />
            <code className={codeClass} />
            <code className={codeClass} />
            <code className={codeClass} />
          </Trans>
        </p>
      </div>
    </div>
  );
}
