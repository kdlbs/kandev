import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import {
  CommentDraftContext,
  type CommentDraftStore,
} from "@/components/task/simple/comment-draft-context";
import { ConversationContent } from "@/app/settings/orchestration/conversation-route";
import { useCoordinatorConversation } from "@/hooks/domains/orchestration/use-coordinator-conversation";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { orchestratorHref, selectedExecutor } from "@/lib/api/domains/orchestration-api";

const drafts = new Map<string, string>();
let draftOwner: string | undefined;
function memoryDrafts(owner: string | undefined, workspace: string): CommentDraftStore {
  if (owner !== draftOwner) {
    drafts.clear();
    draftOwner = owner;
  }
  const key = (task: string) => `${workspace}:${task}`;
  return {
    get: (task) => (owner === draftOwner ? (drafts.get(key(task)) ?? "") : ""),
    set: (task, value) => {
      if (owner !== draftOwner) return;
      if (value) drafts.set(key(task), value);
      else drafts.delete(key(task));
    },
  };
}
function ReadyConversation({ workspaceId, selected }: { workspaceId: string; selected: string }) {
  const { t } = useTranslation();
  const user = useAppStore((s) => s.auth.user?.id);
  const draftStore = useMemo(() => memoryDrafts(user, workspaceId), [user, workspaceId]);
  const { data, error, refresh } = useCoordinatorConversation(workspaceId, selected);
  if (error)
    return (
      <div className="p-4 space-y-3" role="alert">
        <p>{t("orchestration:conversationUnavailable")}</p>
        <Button variant="outline" className="cursor-pointer max-md:min-h-11" onClick={refresh}>
          {t("task:retry")}
        </Button>
      </div>
    );
  if (!data)
    return (
      <p role="status" className="p-4">
        {t("common:loading")}
      </p>
    );
  return (
    <CommentDraftContext.Provider value={draftStore}>
      <ConversationContent key={data.task_id} taskId={data.task_id} embedded />
    </CommentDraftContext.Provider>
  );
}
export function CoordinatorChat({
  catalog,
  selected,
}: {
  catalog: CoordinatorWorkspace;
  selected: string;
}) {
  const { t } = useTranslation();
  const assignment = catalog.assignments.find((item) => item.id === selected);
  if (!assignment)
    return (
      <p className="p-6 text-sm text-muted-foreground">
        {t("orchestration:selectCoordinatorHint")}
      </p>
    );
  const ready =
    catalog.profiles.some((profile) => profile.id === assignment.profile_id) &&
    catalog.executors.some(
      (executor) => executor.id === selectedExecutor(assignment.executor_preference),
    );
  if (!ready)
    return (
      <div className="p-6 space-y-3">
        <p>{t("orchestration:profileUnavailable")}</p>
        <Link
          className="underline cursor-pointer"
          href={orchestratorHref(catalog.workspace.id, assignment.id)}
        >
          {t("orchestration:configureOrchestrator")}
        </Link>
      </div>
    );
  return (
    <ReadyConversation
      key={`${catalog.workspace.id}:${assignment.id}`}
      workspaceId={catalog.workspace.id}
      selected={assignment.id}
    />
  );
}
