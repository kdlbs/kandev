"use client";

import { useState } from "react";
import {
  IconChevronDown,
  IconChevronRight,
  IconCopy,
  IconExternalLink,
  IconMessageQuestion,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { toast } from "@/lib/toast/sonner";
import { formatRelativeTime } from "@/lib/i18n/formats";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { linkToTask } from "@/lib/links";
import {
  inboxHistoryClarificationQuestions,
  inboxHistoryPermissionContent,
  inboxHistoryReasonLabelKey,
  inboxHistorySecondaryText,
} from "@/lib/inbox-history/row-presentation";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";

// Standalone touch icons need both dimensions held to the 44px minimum, not
// just min-height (control-sizing.md, "Standalone touch icons need both
// height and width checks").
const TOUCH_ICON_BUTTON =
  "max-md:min-h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11";

function taskHrefForBundle(bundle: InboxHistoryBundle): string {
  return linkToTask(bundle.task_id, { sessionId: bundle.session_id ?? undefined });
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

// The tab's two actions. Icon-only with an aria-label on a phone or
// coarse-pointer viewport, where the labelled desktop pair does not fit
// alongside the reason badge and asked time without clipping; text-labelled
// otherwise. Rendering one pair rather than both keeps a hidden duplicate
// out of the accessibility tree at every breakpoint.
function InboxHistoryRowActions({
  taskHref,
  onCopyId,
  compact,
}: {
  taskHref: string;
  onCopyId: () => void;
  compact: boolean;
}) {
  const { t } = useTranslation();
  if (compact) {
    return (
      <>
        <Button
          asChild
          variant="outline"
          size="icon"
          className={`cursor-pointer ${TOUCH_ICON_BUTTON}`}
        >
          <Link
            href={taskHref}
            aria-label={t("inboxHistory:openTask")}
            data-testid="inbox-history-open-task"
          >
            <IconExternalLink className="h-4 w-4" />
          </Link>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={`cursor-pointer ${TOUCH_ICON_BUTTON}`}
          onClick={onCopyId}
          aria-label={t("inboxHistory:copyId")}
          data-testid="inbox-history-copy-id"
        >
          <IconCopy className="h-4 w-4" />
        </Button>
      </>
    );
  }
  return (
    <>
      <Button asChild variant="outline" size="sm" className="cursor-pointer">
        <Link href={taskHref} data-testid="inbox-history-open-task">
          {t("inboxHistory:openTask")}
        </Link>
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="cursor-pointer"
        onClick={onCopyId}
        data-testid="inbox-history-copy-id"
      >
        {t("inboxHistory:copyId")}
      </Button>
    </>
  );
}

export function InboxHistoryRow({ bundle }: { bundle: InboxHistoryBundle }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  // Tablet (md-lg) is a coarse-pointer breakpoint (apps/web/AGENTS.md,
  // "Responsive and touch surfaces"): its actions need the same touch
  // treatment as phone, not the fine-pointer desktop pair.
  const compactActions = isMobile || !isFinePointer;

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
      <div className="flex flex-col gap-1.5 px-4 py-2.5 md:flex-row md:items-center md:gap-3">
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
          {expanded ? (
            <IconChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          ) : (
            <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
          )}
        </button>
        {/* Second line on a narrow viewport: the icon/text row above already
            uses the full width, so the reason, asked time and the two
            actions would clip if forced onto one row with it. */}
        <div className="flex min-w-0 items-center justify-between gap-2 pl-[2.75rem] md:shrink-0 md:justify-end md:gap-3 md:pl-0">
          <div className="flex min-w-0 items-center gap-2">
            <Badge variant="secondary" data-testid="inbox-history-row-reason">
              {t(inboxHistoryReasonLabelKey(bundle.reason))}
            </Badge>
            <span
              className="shrink-0 text-xs text-muted-foreground"
              data-testid="inbox-history-row-asked-time"
            >
              {relativeTime}
            </span>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <InboxHistoryRowActions
              taskHref={taskHref}
              onCopyId={handleCopyId}
              compact={compactActions}
            />
          </div>
        </div>
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
