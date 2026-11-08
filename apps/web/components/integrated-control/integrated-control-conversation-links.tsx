import { useTranslation } from "react-i18next";
import { IconMessage, IconPlus } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { controlHref, type ControlView } from "@/lib/navigation/integrated-control";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { useOpenControlConversation } from "@/hooks/domains/coordinator/use-open-control-conversation";
import { useIntegratedControl } from "./integrated-control-provider";
import { openFailureMessages } from "./control-open-failure";

function ConversationLink({
  workspaceId,
  coordinatorId,
  taskId,
  current,
  label,
  view,
  onNavigate,
}: {
  workspaceId: string;
  coordinatorId?: string;
  taskId: string;
  current?: boolean;
  label: string;
  view: ControlView;
  onNavigate?: () => void;
}) {
  const query = useSearchParams();
  const selected =
    query.get("workspace") === workspaceId &&
    (query.get("coordinator") ?? undefined) === coordinatorId &&
    (query.get("task") === taskId || (!query.has("task") && current));
  return (
    <Link
      href={controlHref(view, workspaceId, coordinatorId, current ? undefined : taskId)}
      onNavigated={onNavigate}
      aria-current={selected ? "page" : undefined}
      title={label}
      className="flex min-h-7 cursor-pointer min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-muted aria-[current=page]:bg-accent [@media(pointer:coarse)]:min-h-11"
      data-testid={`control-conversation-${current ? coordinatorId : taskId}`}
    >
      <IconMessage className="size-3.5 shrink-0" aria-hidden />
      <span className="truncate">{label}</span>
    </Link>
  );
}

/**
 * One "Coordinator conversation" entry. With a conversation it links to it;
 * without one it opens a new conversation through the existing coordinator
 * route and then shows it.
 */
export function ControlCoordinatorConversation({
  workspaceId,
  coordinator,
  named,
  canManage,
  view,
  onNavigate,
}: {
  workspaceId: string;
  coordinator: Coordinator;
  named: boolean;
  /** Opening a conversation needs workspace management access, like the conversation screen. */
  canManage: boolean;
  view: ControlView;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const router = useRouter();
  const opener = useOpenControlConversation(workspaceId, coordinator.id, control?.refresh);
  const label = named
    ? t("sidebar:integratedControl.coordinatorConversationNamed", { name: coordinator.name })
    : t("sidebar:integratedControl.coordinatorConversation");
  if (coordinator.conversation_task_id)
    return (
      <ConversationLink
        workspaceId={workspaceId}
        coordinatorId={coordinator.id}
        taskId={coordinator.conversation_task_id}
        current
        view={view}
        onNavigate={onNavigate}
        label={label}
      />
    );
  if (!canManage) return null;
  return (
    <>
      <button
        type="button"
        disabled={opener.pending}
        title={label}
        onClick={async () => {
          if (!(await opener.open())) return;
          router.push(controlHref(view, workspaceId, coordinator.id));
          onNavigate?.();
        }}
        className="flex min-h-7 w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs text-muted-foreground hover:bg-muted disabled:cursor-wait [@media(pointer:coarse)]:min-h-11"
        data-testid={`control-open-conversation-${coordinator.id}`}
      >
        <IconPlus className="size-3.5 shrink-0" aria-hidden />
        <span className="truncate">{label}</span>
      </button>
      {openFailureMessages(opener.failure, t).map((message) => (
        <p key={message} role="alert" className="px-2 py-1 text-xs text-muted-foreground">
          {message}
        </p>
      ))}
    </>
  );
}

/** A workspace's independent conversations, folded to one line with their count. */
export function ControlIndependentConversations({
  workspaceId,
  view,
  onNavigate,
}: {
  workspaceId: string;
  view: ControlView;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const collection = control?.data?.conversationsByWorkspace[workspaceId]?.independent;
  if (!collection?.items.length) return null;
  return (
    <details className="min-w-0" data-testid="control-independent-conversations">
      <summary className="cursor-pointer truncate px-2 py-1.5 text-xs text-muted-foreground [@media(pointer:coarse)]:min-h-11">
        {t("sidebar:integratedControl.independentConversationCount", {
          count: collection.items.length,
        })}
      </summary>
      {collection.items.map((item) => (
        <ConversationLink
          key={item.task_id}
          workspaceId={workspaceId}
          taskId={item.task_id}
          view={view}
          onNavigate={onNavigate}
          label={item.name || t("sidebar:integratedControl.unnamedConversation")}
        />
      ))}
    </details>
  );
}
