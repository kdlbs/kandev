"use client";

import { useEffect, useState } from "react";
import { IconCloud, IconCloudOff } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getWebSocketClient } from "@/lib/ws/connection";

type SessionStatusResponse = {
  remote_name?: string;
  remote_state?: string;
  remote_created_at?: string;
  remote_checked_at?: string;
  remote_status_error?: string;
};

type RemoteCloudTooltipProps = {
  taskId: string;
  sessionId?: string | null;
  fallbackName?: string | null;
  iconClassName?: string;
};

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

export function RemoteCloudTooltip({
  taskId,
  sessionId,
  fallbackName,
  iconClassName = "h-3.5 w-3.5",
}: RemoteCloudTooltipProps) {
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState<SessionStatusResponse | null>(null);
  const [fetchedSessionId, setFetchedSessionId] = useState<string | null>(null);

  useEffect(() => {
    if (!open || !sessionId || fetchedSessionId === sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    client
      .request<SessionStatusResponse>(
        "task.session.status",
        { task_id: taskId, session_id: sessionId },
        10000,
      )
      .then((res) => setStatus(res))
      .catch(() => {})
      .finally(() => setFetchedSessionId(sessionId));
  }, [open, fetchedSessionId, sessionId, taskId]);

  const remoteName = status?.remote_name ?? fallbackName ?? "Remote executor";
  const hasError = Boolean(status?.remote_status_error);
  const loading = Boolean(open && sessionId && fetchedSessionId !== sessionId);
  const Icon = hasError ? IconCloudOff : IconCloud;
  const iconClass = hasError
    ? `${iconClassName} text-destructive`
    : `${iconClassName} text-muted-foreground`;
  const createdAt = formatTimestamp(status?.remote_created_at);
  const checkedAt = formatTimestamp(status?.remote_checked_at);

  return (
    <Tooltip onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <span className="cursor-default">
          <Icon className={iconClass} />
        </span>
      </TooltipTrigger>
      <RemoteCloudStatusContent
        remoteName={remoteName}
        remoteState={status?.remote_state}
        createdAt={createdAt}
        checkedAt={checkedAt}
        remoteStatusError={status?.remote_status_error}
        loading={loading}
      />
    </Tooltip>
  );
}
