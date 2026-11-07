import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLifebuoy, IconMessageCircleQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { NeedsYouInboxRow } from "@/components/needs-you-inbox/needs-you-inbox-row";
import { ClarificationInputOverlay } from "@/components/task/chat/clarification-input-overlay";
import { useLateClarificationMessage } from "@/hooks/use-late-clarification-message";
import { formatRelativeTime } from "@/lib/i18n/formats";
import { linkToTask } from "@/lib/links";
import {
  rowPrimaryText,
  rowQuestionCount,
  rowSecondaryText,
} from "@/lib/needs-you-inbox/row-presentation";
import { sessionDeliveryRecoveryReasonMessage } from "@/lib/services/session-recovery-service";
import type { ClarificationInboxBundle, InboxRecoveryBlock } from "@/lib/types/clarification-inbox";

/** The existing Needs-you row, answered in place through the claim path. */
export function ControlAnswerableRow({
  bundle,
  workspaceId,
}: {
  bundle: ClarificationInboxBundle;
  workspaceId: string;
}) {
  const lateAnswer = useLateClarificationMessage(bundle.messages[0], { workspaceId });
  return (
    <NeedsYouInboxRow
      bundle={bundle}
      onLateAnswer={lateAnswer.send}
      lateAnswerState={lateAnswer.state}
    />
  );
}

/**
 * A question left pending on an earlier turn. It can no longer be claimed, so
 * the answer goes to the session as an ordinary message quoting the question,
 * through the same late-answer path the task transcript uses.
 */
export function ControlEarlierQuestionRow({
  bundle,
  workspaceId,
}: {
  bundle: ClarificationInboxBundle;
  workspaceId: string;
}) {
  const { t } = useTranslation();
  const [answering, setAnswering] = useState(false);
  const scopeRef = useRef<HTMLDivElement>(null);
  const lateAnswer = useLateClarificationMessage(bundle.messages[0], { workspaceId });
  const questionCount = rowQuestionCount(bundle);
  return (
    <div
      ref={scopeRef}
      className="px-4 py-2.5"
      data-testid="control-inbox-earlier-row"
      data-pending-id={bundle.pending_id}
    >
      <div className="flex items-center gap-3">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted">
          <IconMessageCircleQuestion className="h-4 w-4 text-muted-foreground" aria-hidden />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">
            {rowPrimaryText(bundle, t("needsYouInbox:questionFromAgent"))}
          </span>
          <span className="block truncate text-xs text-muted-foreground">
            {rowSecondaryText(bundle)}
            {questionCount > 1 &&
              ` · ${t("needsYouInbox:questionCount", { count: questionCount })}`}
          </span>
        </span>
        <span className="shrink-0 text-xs text-muted-foreground">
          {formatRelativeTime(bundle.created_at)}
        </span>
        {!answering && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0 cursor-pointer max-md:min-h-11"
            onClick={() => {
              if (lateAnswer.state.status === "sent" || lateAnswer.state.status === "queued")
                lateAnswer.reset();
              setAnswering(true);
            }}
            data-testid="control-inbox-earlier-answer"
          >
            {t("task:answerAsNewMessage")}
          </Button>
        )}
        <Button
          asChild
          variant="ghost"
          size="sm"
          className="hidden shrink-0 cursor-pointer sm:inline-flex"
        >
          <Link href={linkToTask(bundle.task_id, { sessionId: bundle.session_id || undefined })}>
            {t("needsYouInbox:openTask")}
          </Link>
        </Button>
      </div>
      {answering && (
        <div className="mt-3 border-l border-border/60 pl-3">
          <ClarificationInputOverlay
            messages={bundle.messages}
            mode="late"
            onLateAnswer={lateAnswer.send}
            initialAnswers={lateAnswer.state.snapshot?.answers}
            lateAnswerState={lateAnswer.state}
            onResolved={() => {}}
            shortcutScopeRef={scopeRef}
            onDismiss={() => setAnswering(false)}
          />
        </div>
      )}
    </div>
  );
}

/** An open recovery block; the decision itself is made on the task chat's recovery card. */
export function ControlRecoveryBlockRow({ block }: { block: InboxRecoveryBlock }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center gap-3 px-4 py-2.5"
      data-testid="control-inbox-recovery-row"
      data-block-id={block.block_id}
    >
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted">
        <IconLifebuoy className="h-4 w-4 text-warning" aria-hidden />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">
          {block.task_title || block.task_id}
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          {sessionDeliveryRecoveryReasonMessage(block.reason, t)}
        </span>
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {formatRelativeTime(block.created_at)}
      </span>
      <Button
        asChild
        variant="outline"
        size="sm"
        className="shrink-0 cursor-pointer max-md:min-h-11"
      >
        <Link
          href={linkToTask(block.task_id, { sessionId: block.session_id || undefined })}
          data-testid="control-inbox-recovery-open"
        >
          {t("sidebar:controlInbox.openDecision")}
        </Link>
      </Button>
    </div>
  );
}
