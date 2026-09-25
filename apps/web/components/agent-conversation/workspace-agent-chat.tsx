"use client";

import { memo, useCallback, useContext, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";
import { generateUUID } from "@/lib/utils";
import {
  dispatchManagedConversation,
  pluginConversationUrl,
  PluginConversationScopeProvider,
  ConversationScopeContext,
  fetchConversationBinding,
} from "@/lib/plugins/conversation-scope";
import { pluginConversationApi } from "@/lib/plugins/conversation-host";
import type { WorkspaceAgentChatProps, WorkspaceAgentChatStatus } from "@kandev/plugin-sdk";

type ManagedDescriptor = {
  taskId: string;
  sessionId: string;
  workspaceId: string;
  managedConversationToken: string;
};

type InternalProps = WorkspaceAgentChatProps & { pluginId?: string };
const permissionDeniedStatus: WorkspaceAgentChatStatus = "permission-denied";

function transcriptErrorStatus(code: string): WorkspaceAgentChatStatus {
  if (code === "not_found") return "deleted";
  if (code === "unauthenticated") return permissionDeniedStatus;
  return "unavailable";
}

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
    let active = true;
    if (!pluginId) {
      setStatus("unavailable");
      return () => {
        active = false;
        controller.abort();
      };
    }
    setDescriptor(null);
    setStatus("loading");
    fetchConversationBinding(pluginId, controller.signal)
      .then((binding) =>
        fetch(
          pluginConversationUrl(
            pluginId,
            `/conversation/managed/${encodeURIComponent(conversationId)}?workspace_id=${encodeURIComponent(workspaceId)}`,
          ),
          {
            credentials: "include",
            cache: "no-store",
            signal: controller.signal,
            headers: { "X-Kandev-Plugin-Binding": binding.bindingToken },
          },
        ),
      )
      .then(async (response) => {
        if (response.ok) return response.json() as Promise<ManagedDescriptor>;
        if (response.status === 403) throw new Error(permissionDeniedStatus);
        if (response.status === 404) throw new Error("deleted");
        throw new Error("unavailable");
      })
      .then((next) => {
        if (
          next.workspaceId !== workspaceId ||
          next.sessionId !== conversationId ||
          !next.managedConversationToken
        )
          throw new Error(permissionDeniedStatus);
        if (!active || controller.signal.aborted) return;
        setDescriptor(next);
        setStatus("ready");
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted)
          setStatus(
            error instanceof Error && [permissionDeniedStatus, "deleted"].includes(error.message)
              ? (error.message as WorkspaceAgentChatStatus)
              : "unavailable",
          );
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [conversationId, pluginId, resourceVersion, workspaceId]);
  return { descriptor, status };
}

// eslint-disable-next-line max-lines-per-function -- The composer and transcript share one bound scope.
function ManagedTranscript({
  pluginId,
  descriptor,
  readOnly,
  placeholderOverride,
  onStatus,
}: {
  pluginId: string;
  descriptor: ManagedDescriptor;
  readOnly: boolean;
  placeholderOverride?: string;
  onStatus(status: WorkspaceAgentChatStatus): void;
}) {
  const { t } = useTranslation();
  const scope = useContext(ConversationScopeContext);
  const { messages, loading, removed, error } = pluginConversationApi.useSessionMessages({
    sessionId: descriptor.sessionId,
    taskId: descriptor.taskId,
    sort: "asc",
    pageSize: 50,
  });
  const [content, setContent] = useState("");
  const [sending, setSending] = useState(false);
  const [sendFailed, setSendFailed] = useState(false);
  useEffect(() => {
    if (removed) onStatus("deleted");
  }, [onStatus, removed]);
  useEffect(() => {
    if (!error) return;
    onStatus(transcriptErrorStatus(error.code));
  }, [error, onStatus]);
  const send = useCallback(async () => {
    if (!content.trim() || sending) return;
    setSending(true);
    setSendFailed(false);
    try {
      const binding = await scope?.ready();
      if (!binding) throw new Error("binding unavailable");
      await dispatchManagedConversation({
        pluginId,
        sessionId: descriptor.sessionId,
        workspaceId: descriptor.workspaceId,
        content,
        occurrenceKey: generateUUID(),
        bindingToken: binding.bindingToken,
      });
      setContent("");
    } catch {
      setSendFailed(true);
    } finally {
      setSending(false);
    }
  }, [content, descriptor, pluginId, scope, sending]);
  if (removed)
    return (
      <div
        data-testid="workspace-agent-chat-status"
        data-status="deleted"
        className="flex h-full items-center justify-center"
      >
        <p role="status">{t("plugins:workspaceAgentChatSessionEnded")}</p>
      </div>
    );
  if (error)
    return (
      <div
        data-testid="workspace-agent-chat-status"
        data-status="unavailable"
        className="flex h-full items-center justify-center"
      >
        <p role="alert">{t("plugins:workspaceAgentChatUnavailable")}</p>
      </div>
    );
  return (
    <div data-testid="workspace-agent-chat" className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3" aria-busy={loading}>
        {messages.map((message) => (
          <div
            key={message.id}
            data-testid={
              message.authorType === "agent" ? "workspace-agent-chat-agent-message" : undefined
            }
            className="rounded-md bg-muted p-3 text-sm"
          >
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
          {sendFailed && <p role="alert">{t("plugins:workspaceAgentChatSendFailed")}</p>}
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
  const [transcriptStatus, setTranscriptStatus] = useState<WorkspaceAgentChatStatus | null>(null);
  useEffect(() => {
    setTranscriptStatus(null);
  }, [conversationId, pluginId, resourceVersion, workspaceId]);
  const effectiveStatus = transcriptStatus ?? status;
  useEffect(() => {
    onStatus?.(effectiveStatus);
  }, [effectiveStatus, onStatus]);
  if (effectiveStatus !== "ready" || !descriptor || !pluginId) {
    let terminalMessage: string | null = null;
    if (effectiveStatus === "deleted")
      terminalMessage = t("plugins:workspaceAgentChatSessionEnded");
    if (effectiveStatus === permissionDeniedStatus)
      terminalMessage = t("plugins:workspaceAgentChatPermissionDenied");
    if (effectiveStatus === "unavailable")
      terminalMessage = t("plugins:workspaceAgentChatUnavailable");
    return (
      <div
        data-testid="workspace-agent-chat-status"
        data-status={effectiveStatus}
        className="flex h-full items-center justify-center"
      >
        {terminalMessage ? (
          <p role={effectiveStatus === "deleted" ? "status" : "alert"}>{terminalMessage}</p>
        ) : (
          <Spinner aria-label={t("plugins:loadingWorkspaceAgentChat")} />
        )}
      </div>
    );
  }
  return (
    <PluginConversationScopeProvider
      pluginId={pluginId}
      taskId={descriptor.taskId}
      sessionId={descriptor.sessionId}
      generation={Number(resourceVersion) || 0}
      managedConversationToken={descriptor.managedConversationToken}
    >
      <ManagedTranscript
        pluginId={pluginId}
        descriptor={descriptor}
        readOnly={readOnly}
        placeholderOverride={placeholderOverride}
        onStatus={setTranscriptStatus}
      />
    </PluginConversationScopeProvider>
  );
});
