"use client";

import { memo, useCallback, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";
import { generateUUID } from "@/lib/utils";
import {
  pluginConversationUrl,
  PluginConversationScopeProvider,
} from "@/lib/plugins/conversation-scope";
import { pluginConversationApi } from "@/lib/plugins/conversation-host";
import type { WorkspaceAgentChatProps, WorkspaceAgentChatStatus } from "@kandev/plugin-sdk";

type ManagedDescriptor = { taskId: string; sessionId: string; workspaceId: string };
type InternalProps = WorkspaceAgentChatProps & { pluginId?: string };

function useManagedDescriptor(
  pluginId: string | undefined,
  workspaceId: string,
  conversationId: string,
  resourceVersion: string,
) {
  const [descriptor, setDescriptor] = useState<ManagedDescriptor | null>(null);
  const [status, setStatus] = useState<WorkspaceAgentChatStatus>("loading");
  useEffect(() => {
    const controller = new AbortController();
    if (!pluginId) {
      setStatus("unavailable");
      return () => controller.abort();
    }
    setDescriptor(null);
    setStatus("loading");
    fetch(
      pluginConversationUrl(
        pluginId,
        `/conversation/managed/${encodeURIComponent(conversationId)}?workspace_id=${encodeURIComponent(workspaceId)}`,
      ),
      { credentials: "include", cache: "no-store", signal: controller.signal },
    )
      .then(async (response) => {
        if (response.ok) return response.json() as Promise<ManagedDescriptor>;
        if (response.status === 403) throw new Error("permission-denied");
        if (response.status === 404) throw new Error("deleted");
        throw new Error("unavailable");
      })
      .then((next) => {
        if (next.workspaceId !== workspaceId || next.sessionId !== conversationId)
          throw new Error("permission-denied");
        setDescriptor(next);
        setStatus("ready");
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted)
          setStatus(
            error instanceof Error && ["permission-denied", "deleted"].includes(error.message)
              ? (error.message as WorkspaceAgentChatStatus)
              : "unavailable",
          );
      });
    return () => controller.abort();
  }, [conversationId, pluginId, resourceVersion, workspaceId]);
  return { descriptor, status };
}

function ManagedTranscript({
  pluginId,
  descriptor,
  readOnly,
  placeholderOverride,
}: {
  pluginId: string;
  descriptor: ManagedDescriptor;
  readOnly: boolean;
  placeholderOverride?: string;
}) {
  const { t } = useTranslation();
  const { messages, loading, removed } = pluginConversationApi.useSessionMessages({
    sessionId: descriptor.sessionId,
    taskId: descriptor.taskId,
    sort: "asc",
    pageSize: 50,
  });
  const [content, setContent] = useState("");
  const [sending, setSending] = useState(false);
  const send = useCallback(async () => {
    if (!content.trim() || sending) return;
    setSending(true);
    try {
      const response = await fetch(
        pluginConversationUrl(
          pluginId,
          `/conversation/managed/${encodeURIComponent(descriptor.sessionId)}/dispatch?workspace_id=${encodeURIComponent(descriptor.workspaceId)}`,
        ),
        {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ content, occurrenceKey: generateUUID() }),
        },
      );
      if (!response.ok) throw new Error("send failed");
      setContent("");
    } finally {
      setSending(false);
    }
  }, [content, descriptor, pluginId, sending]);
  if (removed)
    return (
      <div
        data-testid="workspace-agent-chat-status"
        data-status="deleted"
        className="flex h-full items-center justify-center"
      >
        <Spinner aria-label={t("plugins:loadingWorkspaceAgentChat")} />
      </div>
    );
  return (
    <div data-testid="workspace-agent-chat" className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3" aria-busy={loading}>
        {messages.map((message) => (
          <div key={message.id} className="rounded-md bg-muted p-3 text-sm">
            {message.content}
          </div>
        ))}
      </div>
      {!readOnly && (
        <div className="flex gap-2 border-t p-3">
          <Textarea
            aria-label={t("plugins:workspaceAgentChatInput")}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            placeholder={placeholderOverride}
          />
          <Button type="button" onClick={send} disabled={sending || !content.trim()}>
            {t("plugins:workspaceAgentChatSend")}
          </Button>
        </div>
      )}
    </div>
  );
}

export const WorkspaceAgentChat = memo(function WorkspaceAgentChat({
  pluginId,
  workspaceId,
  conversationId,
  resourceVersion,
  readOnly = false,
  placeholderOverride,
  onStatus,
}: InternalProps) {
  const { t } = useTranslation();
  const { descriptor, status } = useManagedDescriptor(
    pluginId,
    workspaceId,
    conversationId,
    resourceVersion,
  );
  useEffect(() => {
    onStatus?.(status);
  }, [onStatus, status]);
  if (status !== "ready" || !descriptor || !pluginId)
    return (
      <div
        data-testid="workspace-agent-chat-status"
        data-status={status}
        className="flex h-full items-center justify-center"
      >
        <Spinner aria-label={t("plugins:loadingWorkspaceAgentChat")} />
      </div>
    );
  return (
    <PluginConversationScopeProvider
      pluginId={pluginId}
      taskId={descriptor.taskId}
      sessionId={descriptor.sessionId}
      generation={Number(resourceVersion) || 0}
    >
      <ManagedTranscript
        pluginId={pluginId}
        descriptor={descriptor}
        readOnly={readOnly}
        placeholderOverride={placeholderOverride}
      />
    </PluginConversationScopeProvider>
  );
});
