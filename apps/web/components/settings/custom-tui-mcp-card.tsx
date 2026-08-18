"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { MCPStrategySelect, useMCPStrategies } from "@/components/settings/mcp-strategy-select";
import { updateCustomTUIAgentMCPStrategy } from "@/lib/api/domains/settings-api";
import type { Agent } from "@/lib/types/http-agents";

type CustomTUIMcpCardProps = {
  /** Renders nothing unless this is a saved custom TUI agent. */
  agent: Agent | null | undefined;
};

/**
 * Lets the user change a custom TUI agent's MCP injection strategy.
 *
 * Self-contained (own store write, own toast) so it can mount wherever the
 * agent is actually shown. It first lived on the agent detail route, which
 * redirects saved agents straight back to the index — so it rendered nowhere a
 * user would find it.
 *
 * Saves immediately rather than registering with the shared settings-save
 * coordinator, matching the profile-enabled toggle: the strategy is agent-level
 * state outside the profile draft, the backend rebuilds the registry entry as
 * part of the write, and turning it on is what makes the per-profile MCP editor
 * appear — deferring that behind a separate Save would be confusing.
 */
export function CustomTUIMcpCard({ agent }: CustomTUIMcpCardProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  // Gated: this card is rendered once per agent on the index, and hooks run
  // before the early return below, so an unconditional fetch would issue one
  // identical request per built-in agent as well.
  const isCustomTUIAgent = Boolean(agent?.tui_config);
  const strategies = useMCPStrategies(isCustomTUIAgent);
  const [saving, setSaving] = useState(false);

  if (!agent?.tui_config) return null;

  const handleChange = async (strategy: string) => {
    setSaving(true);
    try {
      const updated = await updateCustomTUIAgentMCPStrategy(agent.id, strategy);
      // Write through the store rather than waiting on the WS echo so the
      // control reflects the save immediately. Profiles are preserved: this
      // endpoint only changes agent-level settings.
      setSettingsAgents(
        savedAgents.map((item) =>
          item.id === updated.id ? { ...item, ...updated, profiles: item.profiles } : item,
        ),
      );
    } catch (error) {
      toast({
        title: t("agents:failedToSaveAgent"),
        description: error instanceof Error ? error.message : t("agents:requestFailed"),
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="border-t px-3 py-2.5" data-testid={`agent-mcp-strategy-${agent.name}`}>
      <MCPStrategySelect
        id={`agent-mcp-strategy-${agent.id}`}
        value={agent.tui_config.mcp_strategy}
        onChange={handleChange}
        strategies={strategies}
        disabled={saving}
      />
    </div>
  );
}
