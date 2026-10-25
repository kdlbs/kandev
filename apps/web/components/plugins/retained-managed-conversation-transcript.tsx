"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStoreApi } from "@/components/state-provider";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { requestOlderMessages } from "@/hooks/domains/session/older-message-pagination";
import type { Task } from "@/lib/types/http";

type TranscriptMessage = ReturnType<typeof useSessionMessages>["messages"][number];

function TranscriptBody({
  sessionId,
  loading,
  historyInitialized,
  historyError,
  retryHistory,
  messages,
}: {
  sessionId: string | null;
  loading: boolean;
  historyInitialized: boolean;
  historyError: unknown;
  retryHistory: () => void;
  messages: TranscriptMessage[];
}) {
  const { t } = useTranslation("plugins");
  if (!sessionId) {
    return (
      <p className="p-6 text-center text-sm text-muted-foreground">{t("managedChatNoSession")}</p>
    );
  }
  if (loading && !historyInitialized) {
    return (
      <p role="status" className="p-6 text-center text-sm text-muted-foreground">
        {t("managedChatLoading")}
      </p>
    );
  }
  if (Boolean(historyError) && !historyInitialized) {
    return (
      <div className="flex flex-col items-center gap-2 p-6" role="status">
        <p className="text-sm text-muted-foreground">{t("managedChatTranscriptUnavailable")}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11"
          onClick={retryHistory}
        >
          {t("managedChatRetry")}
        </Button>
      </div>
    );
  }
  if (messages.length === 0) {
    return (
      <p className="m-auto p-6 text-center text-sm text-muted-foreground">
        {t("managedChatEmptyConversation")}
      </p>
    );
  }
  return (
    <ol className="space-y-3 p-3">
      {messages.map((message) => (
        <li key={message.id} className="min-w-0">
          <div className="mb-1 text-xs font-medium text-muted-foreground">
            {message.author_type === "user" ? t("managedChatYou") : t("managedChatAgent")}
          </div>
          <p className="whitespace-pre-wrap break-words rounded-lg bg-muted px-3 py-2 text-sm">
            {message.content}
          </p>
        </li>
      ))}
    </ol>
  );
}

export function RetainedManagedConversationTranscript({
  task,
  sessionId,
}: {
  task: Pick<Task, "title">;
  sessionId: string | null;
}) {
  const { t } = useTranslation("plugins");
  const store = useAppStoreApi();
  const history = useSessionMessages(sessionId);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [olderPageFailed, setOlderPageFailed] = useState(false);
  const messages = useMemo(
    () =>
      history.messages
        .filter(
          (message) =>
            message.type === "message" &&
            (message.author_type === "user" || message.author_type === "agent"),
        )
        .sort((left, right) => left.created_at.localeCompare(right.created_at)),
    [history.messages],
  );
  const loadOlder = useCallback(async () => {
    if (!sessionId || !history.oldestCursor || loadingOlder) return;
    setLoadingOlder(true);
    setOlderPageFailed(false);
    try {
      await requestOlderMessages({
        sessionId,
        cursor: history.oldestCursor,
        limit: 100,
        store,
      });
    } catch {
      setOlderPageFailed(true);
    } finally {
      setLoadingOlder(false);
    }
  }, [history.oldestCursor, loadingOlder, sessionId, store]);

  return (
    <main
      className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden"
      data-testid="managed-retained-transcript"
    >
      <header className="shrink-0 border-b px-4 py-3">
        <h1 className="text-lg font-semibold">{task.title}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t("managedChatRetainedDescription")}</p>
      </header>
      <section
        className="min-h-0 flex-1 overflow-y-auto"
        aria-label={t("managedChatRetainedTitle")}
      >
        {sessionId && history.hasMore && history.oldestCursor ? (
          <div className="sticky top-0 z-10 flex justify-center bg-background/90 p-2 backdrop-blur">
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="min-h-11"
              disabled={loadingOlder}
              onClick={() => void loadOlder()}
            >
              {t("managedChatLoadOlderMessages")}
            </Button>
          </div>
        ) : null}
        <TranscriptBody
          sessionId={sessionId}
          loading={history.isLoading}
          historyInitialized={history.historyInitialized}
          historyError={history.historyError}
          retryHistory={history.retryHistory}
          messages={messages}
        />
        {olderPageFailed ? (
          <p role="status" className="p-3 text-sm text-muted-foreground">
            {t("managedChatTranscriptUnavailable")}
          </p>
        ) : null}
      </section>
    </main>
  );
}
