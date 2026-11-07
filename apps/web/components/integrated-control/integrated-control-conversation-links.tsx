import { useTranslation } from "react-i18next";
import { IconMessage, IconPlus } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { controlHref, type ControlView } from "@/lib/navigation/integrated-control";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { useOpenControlConversation } from "@/hooks/domains/coordinator/use-open-control-conversation";
import { useIntegratedControl } from "./integrated-control-provider";
import { openFailureMessages } from "./control-open-failure";

const ROW_CLASS =
  "flex min-h-7 w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

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
  const query = useSearchParams();
  const opener = useOpenControlConversation(workspaceId, coordinator.id, control?.refresh);
  const href = controlHref(view, workspaceId, coordinator.id);
  const label = named
    ? t("sidebar:integratedControl.coordinatorConversationNamed", { name: coordinator.name })
    : t("sidebar:integratedControl.coordinatorConversation");
  if (coordinator.conversation_task_id) {
    const selected =
      query.get("workspace") === workspaceId && query.get("coordinator") === coordinator.id;
    return (
      <Link
        href={href}
        onNavigated={onNavigate}
        aria-current={selected ? "page" : undefined}
        title={label}
        className={`${ROW_CLASS} aria-[current=page]:bg-accent`}
        data-testid={`control-conversation-${coordinator.id}`}
      >
        <IconMessage className="size-3.5 shrink-0" aria-hidden />
        <span className="truncate">{label}</span>
      </Link>
    );
  }
  if (!canManage) return null;
  return (
    <>
      <button
        type="button"
        disabled={opener.pending}
        title={label}
        onClick={async () => {
          if (!(await opener.open())) return;
          router.push(href);
          onNavigate?.();
        }}
        className={`${ROW_CLASS} text-muted-foreground disabled:cursor-wait`}
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
