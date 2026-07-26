"use client";

import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

import type { AgentUpdateJob, InstallJob } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "refreshing",
]);

function updatePhase(status: AgentUpdateJob["status"] | undefined): string | null {
  switch (status) {
    case "queued":
      return "Update queued…";
    case "resolving":
      return "Checking latest version…";
    case "updating":
      return "Updating runtime…";
    case "refreshing":
      return "Refreshing models…";
    default:
      return null;
  }
}

function updateButtonLabel(status: AgentUpdateJob["status"] | undefined) {
  switch (status) {
    case "queued":
      return "Update queued";
    case "resolving":
      return "Resolving update";
    case "updating":
      return "Updating agent";
    case "refreshing":
      return "Refreshing agent capabilities";
    case "failed":
      return "Retry update";
    default:
      return "Update agent";
  }
}

function VersionSummary({
  runtimeUpdate,
  job,
}: {
  runtimeUpdate: RuntimeUpdate;
  job?: AgentUpdateJob;
}) {
  const current = job?.current_version || runtimeUpdate.current_version || "Unknown";
  if (job?.target_version) {
    return <p className="break-words font-mono text-xs">{`${current} → ${job.target_version}`}</p>;
  }
  if (job?.status === "resolving") {
    return <p className="break-words font-mono text-xs">{`${current} → Checking latest…`}</p>;
  }
  return <p className="break-words text-xs text-muted-foreground">Current version: {current}</p>;
}

function UpdateResult({ agentName, job }: { agentName: string; job?: AgentUpdateJob }) {
  if (!job) return null;
  if (job.status === "succeeded" && job.refresh_error) {
    return (
      <p
        className="break-words text-xs text-amber-600 dark:text-amber-400"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        Runtime updated, but capabilities could not be refreshed: {job.refresh_error}
      </p>
    );
  }
  if (job.status === "succeeded") {
    return (
      <p
        className="break-words text-xs text-green-600 dark:text-green-400"
        role="status"
        data-testid={`agent-update-result-${agentName}`}
      >
        Runtime updated successfully.
      </p>
    );
  }
  if (job.status === "failed") {
    return (
      <p
        className="break-words text-xs text-destructive"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        {job.error || "The runtime update failed."}
      </p>
    );
  }
  return null;
}

export function AgentRuntimeUpdateControl({
  agentName,
  runtimeUpdate,
  job,
  installJob,
  onUpdate,
}: {
  agentName: string;
  runtimeUpdate: RuntimeUpdate;
  job?: AgentUpdateJob;
  installJob?: InstallJob;
  onUpdate: (agentName: string) => void;
}) {
  if (!runtimeUpdate.supported) return null;

  const updateInFlight = Boolean(job && ACTIVE_UPDATE_STATUSES.has(job.status));
  const installInFlight = installJob?.status === "queued" || installJob?.status === "running";
  const disabled = updateInFlight || installInFlight;
  const phase = updatePhase(job?.status);
  const buttonLabel = updateButtonLabel(job?.status);

  return (
    <div className="min-w-0 space-y-2" data-testid={`agent-update-control-${agentName}`}>
      <VersionSummary runtimeUpdate={runtimeUpdate} job={job} />
      {phase && (
        <p
          className="flex items-center gap-1.5 text-xs text-muted-foreground"
          role="status"
          data-testid={`agent-update-phase-${agentName}`}
        >
          <IconLoader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
          <span className="min-w-0 break-words">{phase}</span>
        </p>
      )}
      {installInFlight && (
        <p className="break-words text-xs text-muted-foreground">
          Agent installation is in progress.
        </p>
      )}
      {job?.output && (
        <pre
          data-testid={`agent-update-log-${agentName}`}
          className="max-h-40 overflow-x-hidden overflow-y-auto whitespace-pre-wrap break-words rounded-md bg-muted px-2 py-1.5 font-mono text-xs text-muted-foreground"
        >
          {job.output}
        </pre>
      )}
      <UpdateResult agentName={agentName} job={job} />
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="min-h-11 w-full cursor-pointer whitespace-normal"
        disabled={disabled}
        aria-label={buttonLabel}
        onClick={() => onUpdate(agentName)}
        data-testid={`agent-update-button-${agentName}`}
      >
        {updateInFlight ? (
          <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <IconRefresh className="mr-2 h-4 w-4" />
        )}
        {buttonLabel}
      </Button>
    </div>
  );
}
