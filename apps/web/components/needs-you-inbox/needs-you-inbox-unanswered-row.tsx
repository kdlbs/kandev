"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight, IconMessageCircleQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { ClarificationInputOverlay } from "@/components/task/chat/clarification-input-overlay";
import { useLateClarificationMessage } from "@/hooks/use-late-clarification-message";
import { closeUnansweredClarificationInboxBundle } from "@/lib/api/domains/clarification-inbox-api";
import { ApiError } from "@/lib/api/client";
import { formatRelativeTime } from "@/lib/i18n/formats";
import { linkToTask } from "@/lib/links";
import { rowPrimaryText, rowSecondaryText } from "@/lib/needs-you-inbox/row-presentation";
import { toast } from "@/lib/toast/sonner";
import type { ClarificationInboxUnansweredBundle } from "@/lib/types/clarification-inbox";

export type UnansweredDeliveryTarget = "original" | "task";

/** The session a late answer goes to: the asking session unless the user chose the task's. */
export function resolveUnansweredTargetSessionId(
  bundle: ClarificationInboxUnansweredBundle,
  target: UnansweredDeliveryTarget,
): string {
  if (target === "task" && bundle.send_to_task_session_id) return bundle.send_to_task_session_id;
  return bundle.session_id;
}

function useCloseAction(pendingId: string, onChanged: () => void) {
  const { t } = useTranslation();
  const [closing, setClosing] = useState(false);
  const close = async () => {
    if (closing) return;
    setClosing(true);
    try {
      await closeUnansweredClarificationInboxBundle(pendingId);
      onChanged();
    } catch (error) {
      // 409: another caller already closed, answered, or merged it.
      if (error instanceof ApiError && error.status === 409) onChanged();
      else toast.error(t("needsYouInbox:unansweredCloseFailed"));
    } finally {
      setClosing(false);
    }
  };
  return { closing, close };
}

function DeliveryTargetChoice({
  target,
  onChange,
}: {
  target: UnansweredDeliveryTarget;
  onChange: (target: UnansweredDeliveryTarget) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-2 px-4 pb-2 text-xs"
      role="group"
      aria-label={t("needsYouInbox:unansweredSendTo")}
      data-testid="needs-you-unanswered-target"
    >
      <span className="text-muted-foreground">{t("needsYouInbox:unansweredSendTo")}</span>
      <Button
        type="button"
        size="sm"
        variant={target === "original" ? "secondary" : "ghost"}
        aria-pressed={target === "original"}
        className="cursor-pointer max-md:min-h-11"
        onClick={() => onChange("original")}
        data-testid="needs-you-unanswered-target-original"
      >
        {t("needsYouInbox:unansweredTargetOriginal")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant={target === "task" ? "secondary" : "ghost"}
        aria-pressed={target === "task"}
        className="cursor-pointer max-md:min-h-11"
        onClick={() => onChange("task")}
        data-testid="needs-you-unanswered-target-task"
      >
        {t("needsYouInbox:unansweredTargetTask")}
      </Button>
    </div>
  );
}

/**
 * One question the agent stopped waiting on. Answering sends an ordinary
 * message (never the original tool response); closing hides it without
 * sending anything.
 */
export function NeedsYouInboxUnansweredRow({
  bundle,
  onChanged,
}: {
  bundle: ClarificationInboxUnansweredBundle;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [target, setTarget] = useState<UnansweredDeliveryTarget>("original");
  const scopeRef = useRef<HTMLDivElement>(null);
  const targetSessionId = resolveUnansweredTargetSessionId(bundle, target);
  const lateAnswer = useLateClarificationMessage(bundle.messages[0], { targetSessionId });
  const { closing, close } = useCloseAction(bundle.pending_id, onChanged);
  const taskHref = linkToTask(bundle.task_id, { sessionId: targetSessionId });

  return (
    <div
      ref={scopeRef}
      data-testid="needs-you-unanswered-row"
      data-pending-id={bundle.pending_id}
      className="py-1"
    >
      <div className="flex items-center gap-3 px-4 py-2">
        <button
          type="button"
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 text-left max-md:min-h-11"
          onClick={() => setExpanded((current) => !current)}
          aria-expanded={expanded}
          data-testid="needs-you-unanswered-row-toggle"
        >
          <IconMessageCircleQuestion className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm">
              {rowPrimaryText(bundle, t("needsYouInbox:questionFromAgent"))}
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {rowSecondaryText(bundle)}
            </span>
          </span>
          <span className="shrink-0 text-xs text-muted-foreground">
            {formatRelativeTime(bundle.created_at)}
          </span>
          {expanded ? (
            <IconChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          ) : (
            <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
          )}
        </button>
        <Button asChild variant="ghost" size="sm" className="hidden cursor-pointer sm:inline-flex">
          <Link href={taskHref}>{t("needsYouInbox:openTask")}</Link>
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="shrink-0 cursor-pointer max-md:min-h-11"
          disabled={closing}
          onClick={() => void close()}
          data-testid="needs-you-unanswered-close"
        >
          {t("needsYouInbox:unansweredClose")}
        </Button>
      </div>
      {expanded && (
        <div className="space-y-1 pb-2">
          {bundle.original_session_terminal && (
            <p className="px-4 text-xs text-muted-foreground">
              {t("needsYouInbox:unansweredSessionEnded")}
            </p>
          )}
          {bundle.send_to_task_session_id && (
            <DeliveryTargetChoice target={target} onChange={setTarget} />
          )}
          <div className="px-1">
            <ClarificationInputOverlay
              messages={bundle.messages}
              mode="late"
              onLateAnswer={lateAnswer.send}
              lateAnswerState={lateAnswer.state}
              onResolved={() => {}}
              shortcutScopeRef={scopeRef}
              onDismiss={() => setExpanded(false)}
            />
          </div>
        </div>
      )}
    </div>
  );
}
