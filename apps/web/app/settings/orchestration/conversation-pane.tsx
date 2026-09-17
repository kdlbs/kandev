import { PersonaIdentityContext } from "@/components/task/simple/persona-identity-context";
import { useWorkspaceOrchestrators } from "@/hooks/domains/orchestration/use-orchestrator-conversation";
import { ImplicitTaskLinksContext } from "@/components/task/simple/task-link-context";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { TaskChat } from "@/components/task/simple/task-chat";
import { CommentTransportContext } from "@/components/task/simple/comment-transport";
import { RecoveryTransportContext } from "@/components/task/simple/recovery-transport";
import {
  postConversationComment,
  retryConversation,
} from "@/lib/api/domains/orchestration-conversation-api";
import { ActiveSessionRefProvider } from "@/components/task/simple/components/active-session-ref-context";
import { TopbarWorkingIndicator } from "@/components/task/simple/components/topbar-working-indicator";
import { orchestratorsHref, orchestratorHref } from "@/lib/api/domains/orchestration-api";
import type {
  Task,
  TaskComment,
  TaskActivityEntry,
  TaskSession,
  TimelineEvent,
} from "@/components/task/simple/types";
export function OrchestratorConversationPane({
  task,
  comments,
  sessions,
  timeline,
  onCommentsChanged,
  orchestratorId,
}: {
  task: Pick<Task, "id" | "title" | "workspaceId">;
  comments: TaskComment[];
  activity: TaskActivityEntry[];
  sessions: TaskSession[];
  timeline: TimelineEvent[];
  onCommentsChanged: () => void;
  orchestratorId: string;
}) {
  const { t } = useTranslation();
  const { data } = useWorkspaceOrchestrators(task.workspaceId);
  const persona = data?.orchestrators.find((item) => item.id === orchestratorId);
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  return (
    <PersonaIdentityContext.Provider value={persona ?? null}>
      <ImplicitTaskLinksContext.Provider value={false}>
        <ActiveSessionRefProvider>
          <section
            ref={setScrollParent}
            className="flex-1 min-h-0 overflow-y-auto p-4 md:p-6"
            data-testid="orchestrator-conversation"
          >
            <nav className="flex flex-wrap gap-4 text-sm">
              <Link className="underline" href={orchestratorsHref(task.workspaceId)}>
                {t("orchestration:orchestration")}
              </Link>
              <Link className="underline" href={orchestratorHref(task.workspaceId, orchestratorId)}>
                {t("orchestration:configureOrchestrator")}
              </Link>
              <Link className="underline" href={`/?workspaceId=${task.workspaceId}`}>
                {t("orchestration:workspaceBoard")}
              </Link>
            </nav>
            <h1 className="text-xl font-semibold my-4">{task.title}</h1>
            <TopbarWorkingIndicator taskId={task.id} />
            <RecoveryTransportContext.Provider value={retryConversation}>
              <CommentTransportContext.Provider value={postConversationComment}>
                <TaskChat
                  taskId={task.id}
                  comments={comments}
                  sessions={sessions}
                  timeline={timeline}
                  scrollParent={scrollParent}
                  readOnly={false}
                  onCommentsChanged={onCommentsChanged}
                />
              </CommentTransportContext.Provider>
            </RecoveryTransportContext.Provider>
          </section>
        </ActiveSessionRefProvider>
      </ImplicitTaskLinksContext.Provider>
    </PersonaIdentityContext.Provider>
  );
}
