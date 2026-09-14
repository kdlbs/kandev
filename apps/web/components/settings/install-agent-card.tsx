"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconDownload, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { AgentLogo } from "@/components/agent-logo";
import type { InstallJob } from "@/lib/api";
import type { AvailableAgent } from "@/lib/types/http";

type InstallStatus = InstallJob["status"] | "idle";

/**
 * The install-job `status` values are the wire enum; only the button labels are
 * copy. The rule never inspects this function (`mode: "jsx-only"` skips a
 * non-JSX body), so the labels are resolved through `t` at render.
 */
function installButtonContent(
  t: TFunction,
  status: InstallStatus,
): {
  icon: "spinner" | "download";
  label: string;
} {
  switch (status) {
    case "queued":
      return { icon: "spinner", label: t("agents:installQueued") };
    case "running":
      return { icon: "spinner", label: t("agents:installing") };
    case "failed":
      return { icon: "download", label: t("agents:retry") };
    default:
      return { icon: "download", label: t("agents:install") };
  }
}

function InstallButton({
  agentName,
  status,
  onInstall,
}: {
  agentName: string;
  status: InstallStatus;
  onInstall: (name: string) => void;
}) {
  const { t } = useTranslation();
  const isInFlight = status === "queued" || status === "running";
  const btn = installButtonContent(t, status);
  return (
    <Button
      size="sm"
      onClick={() => onInstall(agentName)}
      disabled={isInFlight}
      className="cursor-pointer"
      data-testid={`install-button-${agentName}`}
    >
      {btn.icon === "spinner" ? (
        <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
      ) : (
        <IconDownload className="h-4 w-4 mr-2" />
      )}
      {btn.label}
    </Button>
  );
}

/**
 * Card shown under "Available to Install" on the Agents settings page. While a
 * job is queued/running it shows a live log streamed via the agent.install.*
 * WS events. On failure it surfaces the install script's output + error.
 */
/**
 * The Install button, or nothing.
 *
 * Installing an agent is an org.config.manage write, so a caller without the
 * scope is offered no button; an agent with no install script has nothing to
 * run. Its own component so the two branches do not count against
 * InstallAgentCard's complexity ceiling.
 */
function InstallAction({
  agent,
  status,
  onInstall,
}: {
  agent: AvailableAgent;
  status: InstallStatus;
  onInstall?: (name: string) => void;
}) {
  if (!agent.install_script || !onInstall) return null;
  return <InstallButton agentName={agent.name} status={status} onInstall={onInstall} />;
}

export function InstallAgentCard({
  agent,
  job,
  scriptSlot,
  onInstall,
}: {
  agent: AvailableAgent;
  job: InstallJob | undefined;
  /** The copy-and-script row, rendered above the Install button by the parent. */
  scriptSlot?: React.ReactNode;
  /** Absent for a caller without org.config.manage. Installing an agent is a
   *  write; the card still renders so they can see what is available. */
  onInstall?: (name: string) => void;
}) {
  const status: InstallStatus = job?.status ?? "idle";
  const failed = status === "failed";
  const showLog = Boolean(job?.output) && (status === "queued" || status === "running" || failed);

  return (
    <Card className="border-dashed" data-testid={`install-card-${agent.name}`}>
      <CardContent className="py-4 flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <AgentLogo agentName={agent.name} size={20} className="shrink-0" />
          <h4 className="font-medium">{agent.display_name}</h4>
        </div>
        {agent.description && (
          <p className="text-xs text-muted-foreground line-clamp-2">{agent.description}</p>
        )}
        {scriptSlot}
        <InstallAction agent={agent} status={status} onInstall={onInstall} />
        {showLog && (
          <pre
            data-testid={`install-log-${agent.name}`}
            className={
              "max-h-40 overflow-auto whitespace-pre-wrap rounded-md px-2 py-1.5 font-mono text-xs " +
              (failed ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")
            }
          >
            {job?.output}
          </pre>
        )}
        {failed && job?.error && (
          <p className="text-xs text-destructive" data-testid={`install-error-${agent.name}`}>
            {job.error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
