"use client";

import { useEffect, useState } from "react";
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

const PRESETS = [
  { label: "5 min", expression: "@every 5m" },
  { label: "15 min", expression: "@every 15m" },
  { label: "30 min", expression: "@every 30m" },
  { label: "1 hour", expression: "@hourly" },
  { label: "6 hours", expression: "@every 6h" },
  { label: "Daily", expression: "@daily" },
  { label: "Weekly", expression: "@weekly" },
] as const;

export function ScheduleSelector({ config, isDirty = false, onChange }: ScheduleSelectorProps) {
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
      setError("Invalid expression. Use @every with a duration, a shorthand, or a 5-field cron.");
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
            {preset.label}
          </Button>
        ))}
        <Tooltip>
          <TooltipTrigger asChild>
            <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground ml-1" />
          </TooltipTrigger>
          <TooltipContent className="max-w-[280px]">
            How often to check for matching events. Checked every 30 seconds by a background
            process. Schedules persist across backend restarts.
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="space-y-1">
        <Label className="text-xs text-muted-foreground">Custom interval</Label>
        <Input
          value={customInput}
          onChange={(e) => {
            setCustomInput(e.target.value);
            if (error) setError(null);
          }}
          onBlur={handleCustomBlur}
          data-testid="schedule-custom-input"
          data-settings-dirty={isDirty}
          placeholder="@every 2h30m"
          className={`font-mono text-sm max-w-xs ${error ? "border-destructive" : ""}`}
        />
        {error && <p className="text-xs text-destructive">{error}</p>}
        <p className="text-xs text-muted-foreground">
          Use <code className="bg-muted px-1 rounded">@every</code> with a duration (e.g.,{" "}
          <code className="bg-muted px-1 rounded">@every 10m</code>,{" "}
          <code className="bg-muted px-1 rounded">@every 2h30m</code>) or shorthands like{" "}
          <code className="bg-muted px-1 rounded">@hourly</code>,{" "}
          <code className="bg-muted px-1 rounded">@daily</code>,{" "}
          <code className="bg-muted px-1 rounded">@weekly</code>.
        </p>
      </div>
    </div>
  );
}
