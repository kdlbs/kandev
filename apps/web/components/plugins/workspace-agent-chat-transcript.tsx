"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { pluginConversationApi } from "@/lib/plugins/conversation-host";

export function WorkspaceAgentChatTranscript({
  taskId,
  sessionId,
}: {
  taskId: string;
  sessionId: string | null;
}) {
  const { t } = useTranslation("plugins");
  const history = pluginConversationApi.useSessionMessages({
    taskId,
    sessionId,
    authorTypes: ["user", "agent"],
    sort: "asc",
    pageSize: 50,
  });

  if (!sessionId) {
    return <p className="p-4 text-sm text-muted-foreground">{t("managedChatNoSession")}</p>;
  }
  if (history.loading && !history.hydrated) {
    return (
      <p role="status" className="p-4 text-sm text-muted-foreground">
        {t("managedChatLoading")}
      </p>
    );
  }
  if (history.error && !history.hydrated) {
    return (
      <div className="flex flex-col items-start gap-2 p-4" role="alert">
        <p className="text-sm text-muted-foreground">{t("managedChatTranscriptUnavailable")}</p>
        {history.error.retryable ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11"
            onClick={history.retry}
          >
            {t("managedChatRetry")}
          </Button>
        ) : null}
      </div>
    );
  }

  return (
    <div
      className="flex min-h-0 flex-1 flex-col overflow-y-auto"
      data-testid="managed-chat-transcript"
    >
      {history.hasMore ? (
        <div className="sticky top-0 z-10 flex justify-center bg-background/90 p-2 backdrop-blur">
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11"
            disabled={history.loadingMore}
            onClick={() => void history.loadMore()}
          >
            {t("managedChatLoadOlderMessages")}
          </Button>
        </div>
      ) : null}
      {history.messages.length === 0 ? (
        <p className="m-auto p-6 text-center text-sm text-muted-foreground">
          {t("managedChatEmptyConversation")}
        </p>
      ) : (
        <ol className="space-y-3 p-3">
          {history.messages.map((message) => (
            <li key={message.id} className="min-w-0">
              <div className="mb-1 text-xs font-medium text-muted-foreground">
                {message.authorType === "user" ? t("managedChatYou") : t("managedChatAgent")}
              </div>
              <p className="whitespace-pre-wrap break-words rounded-lg bg-muted px-3 py-2 text-sm">
                {message.content}
              </p>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
