"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type ScheduledConfigProps = {
  config: Record<string, unknown>;
  onUpdate: (config: Record<string, unknown>) => void;
};

const PRESETS = [
  { label: "Every hour", expression: "@hourly" },
  { label: "Every day", expression: "@daily" },
  { label: "Every week", expression: "@weekly" },
] as const;

export function ScheduledConfig({ config, onUpdate }: ScheduledConfigProps) {
  const [cronExpression, setCronExpression] = useState((config.cron_expression as string) ?? "");

  const handlePreset = (expression: string) => {
    setCronExpression(expression);
    onUpdate({ ...config, cron_expression: expression });
  };

  const handleCustomChange = (value: string) => {
    setCronExpression(value);
  };

  const handleCustomBlur = () => {
    onUpdate({ ...config, cron_expression: cronExpression });
  };

  return (
    <div className="space-y-3">
      <div className="flex gap-2">
        {PRESETS.map((preset) => (
          <Button
            key={preset.expression}
            variant={cronExpression === preset.expression ? "secondary" : "outline"}
            size="sm"
            className="cursor-pointer"
            onClick={() => handlePreset(preset.expression)}
          >
            {preset.label}
          </Button>
        ))}
      </div>
      <div className="space-y-1.5">
        <Label className="text-xs">Cron expression</Label>
        <Input
          value={cronExpression}
          onChange={(e) => handleCustomChange(e.target.value)}
          onBlur={handleCustomBlur}
          placeholder="*/5 * * * *"
          className="font-mono text-sm"
        />
        <p className="text-xs text-muted-foreground">
          Standard 5-field cron or shortcuts (@hourly, @daily, @weekly, @every 5m)
        </p>
      </div>
    </div>
  );
}
