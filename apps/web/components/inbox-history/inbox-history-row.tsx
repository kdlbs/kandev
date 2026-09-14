"use client";

import { useState } from "react";
import {
  IconChevronDown,
  IconChevronRight,
  IconMessageQuestion,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { toast } from "@/lib/toast/sonner";
import { formatRelativeTime } from "@/lib/i18n/formats";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import {
  inboxHistoryClarificationQuestions,
  inboxHistoryPermissionContent,
  inboxHistoryReasonLabelKey,
  inboxHistorySecondaryText,
} from "@/lib/inbox-history/row-presentation";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";

function taskHrefForBundle(bundle: InboxHistoryBundle): string {
  if (!bundle.session_id) return `/t/${bundle.task_id}`;
  return `/t/${bundle.task_id}?sessionId=${encodeURIComponent(bundle.session_id)}`;
}

function InboxHistoryQuestionDetail({ bundle }: { bundle: InboxHistoryBundle }) {
  const { t } = useTranslation();
  if (bundle.kind === "permission") {
    const { content, optionNames } = inboxHistoryPermissionContent(bundle);
    return (
      <div className="space-y-1 text-sm" data-testid="inbox-history-permission-detail">
        <p>{content || t("inboxHistory:noContent")}</p>
        {optionNames.length > 0 && (
          <p className="text-xs text-muted-foreground">{optionNames.join(", ")}</p>
        )}
      </div>
    );
  }
  const questions = inboxHistoryClarificationQuestions(bundle);
  return (
    <div className="space-y-3" data-testid="inbox-history-clarification-detail">
      {bundle.context && <p className="text-xs text-muted-foreground">{bundle.context}</p>}
      {questions.map((question) => (
        <div key={question.id} className="text-sm">
          <p className="font-medium">
            {question.title || question.prompt || t("inboxHistory:noContent")}
          </p>
          {question.title && question.prompt && (
            <p className="text-xs text-muted-foreground">{question.prompt}</p>
          )}
          {question.optionLabels.length > 0 && (
            <p className="text-xs text-muted-foreground">{question.optionLabels.join(", ")}</p>
          )}
        </div>
      ))}
    </div>
  );
}

function InboxHistoryRowIcon({ bundle }: { bundle: InboxHistoryBundle }) {
  const { t } = useTranslation();
  if (bundle.kind === "permission") {
    return (
      <span
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted"
        role="img"
        aria-label={t("inboxHistory:permissionLabel")}
        data-testid="inbox-history-permission-icon"
      >
        <IconShieldQuestion className="h-4 w-4 text-amber-500" />
      </span>
    );
  }
  return (
    <span
      className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted"
      role="img"
      aria-label={t("inboxHistory:clarificationLabel")}
    >
      <IconMessageQuestion className="h-4 w-4 text-yellow-500" />
    </span>
  );
}

function inboxHistoryPrimaryText(bundle: InboxHistoryBundle, fallback: string): string {
  if (bundle.kind === "permission") {
    return inboxHistoryPermissionContent(bundle).content || fallback;
  }
  const questions = inboxHistoryClarificationQuestions(bundle);
  return questions[0]?.title || questions[0]?.prompt || bundle.context || fallback;
}

export function InboxHistoryRow({ bundle }: { bundle: InboxHistoryBundle }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);

  const primaryText = inboxHistoryPrimaryText(bundle, t("inboxHistory:noContent"));
  const secondaryText = inboxHistorySecondaryText(bundle);
  const taskHref = taskHrefForBundle(bundle);
  const relativeTime = formatRelativeTime(bundle.created_at);

  const handleCopyId = () => {
    void copyToClipboard(bundle.pending_id).then((ok) => {
      if (ok) toast(t("inboxHistory:copyIdSuccess"));
      else toast.error(t("inboxHistory:copyIdFailed"));
    });
  };

  return (
    <div data-testid="inbox-history-row" data-pending-id={bundle.pending_id}>
      <div className="flex items-center gap-3 px-4 py-2.5">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-3 text-left cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={() => setExpanded((current) => !current)}
          aria-expanded={expanded}
          data-testid="inbox-history-row-toggle"
        >
          <InboxHistoryRowIcon bundle={bundle} />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{primaryText}</span>
            <span className="block truncate text-xs text-muted-foreground">{secondaryText}</span>
          </span>
          <Badge variant="secondary" data-testid="inbox-history-row-reason">
            {t(inboxHistoryReasonLabelKey(bundle.reason))}
          </Badge>
          <span
            className="shrink-0 text-xs text-muted-foreground"
            data-testid="inbox-history-row-asked-time"
          >
            {relativeTime}
          </span>
          {expanded ? (
            <IconChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          ) : (
            <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
          )}
        </button>
        {/* AC .14: opening the owning task is the only navigating action. */}
        <Button
          asChild
          variant="outline"
          size="sm"
          className="hidden shrink-0 cursor-pointer sm:inline-flex"
        >
          <Link href={taskHref} data-testid="inbox-history-open-task">
            {t("inboxHistory:openTask")}
          </Link>
        </Button>
        {/* AC .14: copying the bundle identifier is the only other action. */}
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="shrink-0 cursor-pointer"
          onClick={handleCopyId}
          data-testid="inbox-history-copy-id"
        >
          {t("inboxHistory:copyId")}
        </Button>
      </div>
      {expanded && (
        <div className="space-y-2 px-4 pb-3 pl-[3.25rem]" data-testid="inbox-history-row-detail">
          <p className="text-xs text-muted-foreground" data-testid="inbox-history-turn-identity">
            {bundle.reason === "superseded" && bundle.superseding_turn_id
              ? t("inboxHistory:turnSupersededBy", {
                  askingTurn: bundle.asking_turn_id,
                  supersedingTurn: bundle.superseding_turn_id,
                })
              : t("inboxHistory:turnAsked", { turn: bundle.asking_turn_id })}
          </p>
          {bundle.step_starts_no_agent === true && (
            <p
              className="text-xs text-muted-foreground"
              data-testid="inbox-history-step-starts-no-agent"
            >
              {t("inboxHistory:stepStartsNoAgent")}
            </p>
          )}
          <InboxHistoryQuestionDetail bundle={bundle} />
        </div>
      )}
    </div>
  );
}
