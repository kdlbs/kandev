import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { OrchestratorConversationPane } from "@/app/settings/orchestration/conversation-pane";
import { mapConversationSession } from "@/app/settings/orchestration/conversation-route";
import { CommentDraftContext } from "@/components/task/simple/comment-draft-context";
import { useAssistantConversation } from "@/hooks/domains/orchestration/use-assistant-conversation";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
export function AssistantChat({
  binding,
  revision,
  unavailable = false,
  onAccepted,
}: {
  binding: AssistantBinding;
  revision: number;
  unavailable?: boolean;
  onAccepted: () => void;
}) {
  const { t } = useTranslation();
  const chat = useAssistantConversation(binding, revision);
  const drafts = useMemo(() => {
    const values = new Map<string, string>();
    return {
      get: (id: string) => values.get(id) ?? "",
      set: (id: string, value: string) => {
        if (value) values.set(id, value);
        else values.delete(id);
      },
    };
  }, [binding.id, binding.version]);
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="assistant-chat">
      {chat.error && (
        <div role="alert" className="px-4 py-2 text-sm">
          {t("orchestration:assistantUnavailable")}{" "}
          <Button
            variant="ghost"
            className="cursor-pointer max-md:min-h-11"
            onClick={() => void chat.refresh()}
          >
            {t("task:retry")}
          </Button>
        </div>
      )}
      {!chat.metadata && !chat.error && (
        <p className="p-4" role="status">
          {t("common:loading")}
        </p>
      )}
      {chat.nextCursor && (
        <Button
          variant="ghost"
          className="cursor-pointer max-md:min-h-11 shrink-0"
          disabled={chat.loading}
          onClick={() => void chat.loadMore()}
        >
          {t("orchestration:assistantEarlierMessages")}
        </Button>
      )}
      {chat.metadata && (
        <CommentDraftContext.Provider value={drafts}>
          <OrchestratorConversationPane
            embedded
            transport={async (id, body) => {
              const result = await chat.transport(id, body);
              onAccepted();
              return result;
            }}
            readOnly={
              unavailable ||
              binding.paused ||
              Boolean(chat.error) ||
              Boolean(binding.authority_reason) ||
              Boolean(binding.authority?.unsupported_reason)
            }
            task={{
              id: chat.metadata.task.id,
              title: chat.metadata.task.title,
              workspaceId: chat.metadata.task.workspace_id,
            }}
            orchestratorId={binding.orchestrator_id}
            comments={chat.comments}
            sessions={chat.metadata.sessions.map(mapConversationSession)}
            timeline={[]}
            activity={[]}
            onCommentsChanged={() => void chat.refresh()}
          />
        </CommentDraftContext.Provider>
      )}
    </div>
  );
}
