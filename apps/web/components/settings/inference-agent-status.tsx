"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { InferenceAgent, InferenceAgentStatus } from "@/lib/api/domains/utility-api";

type Props = {
  /**
   * The agent whose status to render. `null`/`undefined` means the agent
   * selected in the form is not present in the latest `/inference-agents`
   * response — e.g. the user picked Claude but Claude was just uninstalled,
   * or the cache hasn't caught up yet. The fallback note is "agent is no
   * longer available".
   */
  agent: InferenceAgent | null | undefined;
  /**
   * Fallback agent display name for the not-available case. Without it the
   * note would read "Agent is no longer available" with no identifier.
   */
  fallbackName?: string;
  /**
   * Re-probe handler. Called when the user clicks Refresh. The parent owns
   * loading state on the InferenceAgent list; this component only manages
   * the in-flight spinner on the button itself so simultaneous clicks
   * don't double-fire.
   */
  onRefresh: () => Promise<unknown> | void;
};

// This switch holds no JSX, so the literal guard never inspected it — the copy
// travels as a catalog key and the caller resolves it at render.
type Note = { key: string; refreshable: boolean };

function noteForStatus(status: InferenceAgentStatus | undefined): Note {
  switch (status) {
    case "probing":
      return { key: "settings:inferenceAgentProbing", refreshable: true };
    case "auth_required":
      return { key: "settings:inferenceAgentAuthRequired", refreshable: true };
    case "not_installed":
      return { key: "settings:inferenceAgentNotInstalled", refreshable: true };
    case "not_configured":
      return { key: "settings:inferenceAgentNotConfigured", refreshable: false };
    case "failed":
      return { key: "settings:inferenceAgentProbeFailed", refreshable: true };
    case "ok":
    default:
      return { key: "settings:inferenceAgentNoModels", refreshable: true };
  }
}

function RefreshButton({ onRefresh }: { onRefresh: () => Promise<unknown> | void }) {
  const { t } = useTranslation();
  const [refreshing, setRefreshing] = useState(false);
  const handleClick = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await onRefresh();
    } finally {
      setRefreshing(false);
    }
  };
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-6 px-2 cursor-pointer"
      onClick={handleClick}
      disabled={refreshing}
      data-testid="inference-agent-refresh"
    >
      <IconRefresh className={refreshing ? "h-3 w-3 animate-spin" : "h-3 w-3"} />
      <span className="ml-1">
        {refreshing ? t("settings:inferenceAgentRefreshing") : t("settings:inferenceAgentRefresh")}
      </span>
    </Button>
  );
}

// Treat a missing `status` field as healthy when models are present —
// older backends (or non-OK API consumers) that don't populate `status`
// should not show a spurious warning. Real probe states ("auth_required",
// "probing", …) still fall through to noteForStatus. Per cubic review.
function isAgentHealthy(agent: InferenceAgent | null | undefined): boolean {
  if (!agent || (agent.models?.length ?? 0) === 0) return false;
  return agent.status === "ok" || agent.status === undefined;
}

export function InferenceAgentStatusNote({ agent, fallbackName, onRefresh }: Props) {
  const { t } = useTranslation();
  if (isAgentHealthy(agent)) {
    return null;
  }

  // `display_name` and the agent id are wire values; only the last-resort
  // fallback is copy.
  const name = agent?.display_name ?? fallbackName ?? t("settings:inferenceAgentThisAgent");
  const note: Note = agent
    ? noteForStatus(agent.status)
    : { key: "settings:inferenceAgentUnavailable", refreshable: true };
  const detail = agent?.status_message?.trim();

  return (
    <div
      className="flex items-start justify-between gap-2 text-xs text-muted-foreground"
      data-testid="inference-agent-status-note"
    >
      <div className="space-y-0.5 min-w-0">
        <p>{t(note.key, { name })}</p>
        {detail && <p className="text-[11px] opacity-80 truncate">{detail}</p>}
      </div>
      {note.refreshable && <RefreshButton onRefresh={onRefresh} />}
    </div>
  );
}
