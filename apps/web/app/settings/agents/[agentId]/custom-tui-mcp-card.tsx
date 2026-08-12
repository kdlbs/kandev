"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { MCPStrategySelect, useMCPStrategies } from "@/components/settings/mcp-strategy-select";
import { updateCustomTUIAgentMCPStrategy } from "@/lib/api/domains/settings-api";
import type { Agent } from "@/lib/types/http-agents";

type CustomTUIMcpCardProps = {
  /** Renders nothing unless this is a saved custom TUI agent. */
  agent: Agent | null | undefined;
  onAgentUpdated: (agent: Agent) => void;
  onToastError: (error: unknown) => void;
};

/**
 * Lets the user change a custom TUI agent's MCP injection strategy.
 *
 * This saves immediately rather than registering with the shared settings-save
 * coordinator, matching the profile-enabled toggle: the strategy is agent-level
 * state that lives outside the profile draft, and the backend rebuilds the
 * registry entry as part of the write, so there is no meaningful draft state to
 * hold. Turning it on is also what makes the per-profile MCP editor appear, so
 * deferring it behind a separate Save would be confusing.
 */
export function CustomTUIMcpCard({ agent, onAgentUpdated, onToastError }: CustomTUIMcpCardProps) {
  const { t } = useTranslation();
  const strategies = useMCPStrategies(true);
  const [saving, setSaving] = useState(false);

  if (!agent?.tui_config) return null;

  const handleChange = async (strategy: string) => {
    setSaving(true);
    try {
      onAgentUpdated(await updateCustomTUIAgentMCPStrategy(agent.id, strategy));
    } catch (error) {
      onToastError(error);
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsCard>
      <CardHeader>
        <CardTitle>{t("agents:mcpToolsTitle")}</CardTitle>
        <CardDescription>{t("agents:mcpToolsDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <MCPStrategySelect
          id={`agent-mcp-strategy-${agent.id}`}
          value={agent.tui_config.mcp_strategy}
          onChange={handleChange}
          strategies={strategies}
          disabled={saving}
        />
      </CardContent>
    </SettingsCard>
  );
}
