"use client";

import { useEffect, useState } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getWebSocketClient } from "@/lib/ws/connection";
import { getExecutorStatusIcon } from "@/lib/executor-icons";

type RemoteStatusData = {
  remote_name?: string;
  remote_state?: string;
  remote_created_at?: string;
  remote_checked_at?: string;
  remote_status_error?: string;
};

type RemoteCloudTooltipProps = {
  taskId: string;
  sessionId?: string | null;
  executorType?: string | null;
  fallbackName?: string | null;
  iconClassName?: string;
  /** When provided, uses this data directly instead of fetching via WS on hover. */
  status?: RemoteStatusData | null;
};

const CONNECTED_THRESHOLD_MS = 2 * 60 * 1000;

function getCloudState(status: RemoteStatusData | null): "connected" | "error" | "stale" {
  if (status?.remote_status_error) return "error";
  if (!status?.remote_checked_at) return "stale";
  const elapsed = Date.now() - new Date(status.remote_checked_at).getTime();
  if (elapsed < CONNECTED_THRESHOLD_MS) return "connected";
  return "stale";
}

function formatTimestamp(value?: string): string | null {
  if (!value) return null;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleString();
}

function RemoteCloudStatusContent({
  remoteName,
  remoteState,
  createdAt,
  checkedAt,
  remoteStatusError,
  loading,
}: {
  remoteName: string;
  remoteState?: string;
  createdAt: string | null;
  checkedAt: string | null;
  remoteStatusError?: string;
  loading: boolean;
}) {
  return (
    <TooltipContent side="top" className="space-y-0.5">
      <div className="font-medium">{remoteName}</div>
      {remoteState && <div>State: {remoteState}</div>}
      {createdAt && <div>Created: {createdAt}</div>}
      {checkedAt && <div>Last check: {checkedAt}</div>}
      {remoteStatusError && (
        <div className="text-destructive">Status failed: {remoteStatusError}</div>
      )}
      {loading && <div>Loading status...</div>}
    </TooltipContent>
  );
}

const CLOUD_STATE_CLASSES: Record<ReturnType<typeof getCloudState>, string> = {
  error: "text-destructive",
  connected: "text-emerald-500",
  stale: "text-muted-foreground",
};

export function RemoteCloudTooltip({
  taskId,
  sessionId,
  executorType,
  fallbackName,
  iconClassName = "h-3.5 w-3.5",
  status: externalStatus,
}: RemoteCloudTooltipProps) {
  const hasExternalStatus = externalStatus !== undefined;
  const [open, setOpen] = useState(false);
  const [fetchedStatus, setFetchedStatus] = useState<RemoteStatusData | null>(null);
  const [fetchedSessionId, setFetchedSessionId] = useState<string | null>(null);

  useEffect(() => {
    if (hasExternalStatus) return;
    if (!open || !sessionId || fetchedSessionId === sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    client
      .request<RemoteStatusData>(
        "task.session.status",
        { task_id: taskId, session_id: sessionId },
        10000,
      )
      .then((res) => setFetchedStatus(res))
      .catch(() => {})
      .finally(() => setFetchedSessionId(sessionId));
  }, [hasExternalStatus, open, fetchedSessionId, sessionId, taskId]);

  const status = hasExternalStatus ? (externalStatus ?? null) : fetchedStatus;
  const remoteName = status?.remote_name ?? fallbackName ?? "Remote executor";
  const cloudState = getCloudState(status);
  const loading = Boolean(
    !hasExternalStatus && open && sessionId && fetchedSessionId !== sessionId,
  );
  const icon = getExecutorStatusIcon(executorType, cloudState === "error");
  const Icon = icon.Icon;

  return (
    <Tooltip onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <span className="cursor-default">
          <Icon
            data-testid={icon.testId}
            className={`${iconClassName} ${CLOUD_STATE_CLASSES[cloudState]}`}
          />
        </span>
      </TooltipTrigger>
      <RemoteCloudStatusContent
        remoteName={remoteName}
        remoteState={status?.remote_state}
        createdAt={formatTimestamp(status?.remote_created_at)}
        checkedAt={formatTimestamp(status?.remote_checked_at)}
        remoteStatusError={status?.remote_status_error}
        loading={loading}
      />
    </Tooltip>
  );
}
