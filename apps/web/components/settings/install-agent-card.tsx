"use client";

import { IconDownload, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { AgentLogo } from "@/components/agent-logo";
import type { InstallJob } from "@/lib/api";
import type { AvailableAgent } from "@/lib/types/http";

type InstallStatus = InstallJob["status"] | "idle";

function installButtonContent(status: InstallStatus): {
  icon: "spinner" | "download";
  label: string;
} {
  switch (status) {
    case "queued":
      return { icon: "spinner", label: "Queued…" };
    case "running":
      return { icon: "spinner", label: "Installing…" };
    case "failed":
      return { icon: "download", label: "Retry" };
    default:
      return { icon: "download", label: "Install" };
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
  const isInFlight = status === "queued" || status === "running";
  const btn = installButtonContent(status);
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
  onInstall: (name: string) => void;
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
        {agent.install_script && (
          <InstallButton agentName={agent.name} status={status} onInstall={onInstall} />
        )}
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
