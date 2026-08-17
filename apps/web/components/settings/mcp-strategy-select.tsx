"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { listMCPStrategies } from "@/lib/api/domains/settings-api";
import type { MCPStrategyOption } from "@/lib/types/http-agents";

/**
 * Radix Select cannot hold an empty-string value, so the "off" choice needs a
 * sentinel. Callers convert between it and "" at the API boundary via
 * {@link toStrategyValue} / {@link fromStrategyValue}. It is a control value,
 * never displayed and never translated.
 */
export const MCP_STRATEGY_NONE = "__none__";

/** Maps a stored strategy key ("" = off) to the Select's value. */
export function toStrategyValue(strategy: string | undefined): string {
  return strategy ? strategy : MCP_STRATEGY_NONE;
}

/** Maps the Select's value back to a stored strategy key ("" = off). */
export function fromStrategyValue(value: string): string {
  return value === MCP_STRATEGY_NONE ? "" : value;
}

/**
 * Loads the selectable MCP strategies from the backend.
 *
 * The list is fetched rather than hardcoded so a strategy added in Go becomes
 * selectable without a matching frontend edit — and so an option can never be
 * offered that the backend would reject on save.
 */
export function useMCPStrategies(enabled: boolean) {
  const [strategies, setStrategies] = useState<MCPStrategyOption[]>([]);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    listMCPStrategies()
      .then((options) => {
        if (active) setStrategies(options);
      })
      .catch(() => {
        // Non-fatal: the picker falls back to the "off" choice only. Creating
        // an agent without MCP still works, which is the pre-existing behavior.
        if (active) setStrategies([]);
      });
    return () => {
      active = false;
    };
  }, [enabled]);

  return strategies;
}

type MCPStrategySelectProps = {
  id: string;
  /** Stored strategy key; "" or undefined means no injection. */
  value: string | undefined;
  /** Receives the stored strategy key ("" when the user picks "off"). */
  onChange: (strategy: string) => void;
  strategies: MCPStrategyOption[];
  disabled?: boolean;
};

/**
 * Picks how kandev injects its per-session MCP server into a custom TUI agent's
 * wrapped CLI.
 *
 * This is an explicit choice rather than something inferred from the command,
 * because the command is arbitrary (`zsh -ic "fuelclaude --model opus"` is a
 * real example) and a wrong guess fails silently: kandev writes a config file,
 * the CLI ignores it, and no error surfaces anywhere.
 */
export function MCPStrategySelect({
  id,
  value,
  onChange,
  strategies,
  disabled,
}: MCPStrategySelectProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{t("agents:mcpStrategy")}</Label>
      <Select
        value={toStrategyValue(value)}
        onValueChange={(next) => onChange(fromStrategyValue(next))}
        disabled={disabled}
      >
        <SelectTrigger id={id} className="cursor-pointer" data-testid="mcp-strategy-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={MCP_STRATEGY_NONE} className="cursor-pointer">
            {t("agents:mcpStrategyNone")}
          </SelectItem>
          {strategies.map((option) => (
            <SelectItem key={option.key} value={option.key} className="cursor-pointer">
              {t("agents:mcpStrategyOption", {
                name: option.key,
                mechanism: option.description,
              })}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("agents:mcpStrategyHelp")}</p>
    </div>
  );
}
