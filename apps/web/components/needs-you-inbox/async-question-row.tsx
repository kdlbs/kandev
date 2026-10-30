"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconArrowLeft,
  IconChevronDown,
  IconChevronRight,
  IconClockPause,
  IconMessageQuestion,
  IconShare,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { toast } from "@/lib/toast/sonner";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { formatRelativeTime } from "@/lib/i18n/formats";
import {
  respondToAgentQuestion,
  type AgentQuestionAnswer,
} from "@/lib/api/domains/agent-questions-api";
import { dismissClarificationInboxBundle } from "@/lib/api/domains/clarification-inbox-api";
import type {
  ClarificationInboxAsyncQuestion,
  ClarificationInboxAsyncOutcome,
} from "@/lib/types/clarification-inbox";
import {
  ASYNC_SIDECAR_PREFIX,
  asyncQuestionPrimaryText,
} from "@/lib/needs-you-inbox/async-question-groups";
import { AsyncQuestionActions, AsyncQuestionForm } from "./async-question-form";

type Draft = {
  selections: Record<string, string>;
  customText: Record<string, string>;
};

// Width changes unmount the desktop expansion and the phone surface, so the
// unsent draft lives above both presentations.
const drafts = new Map<string, Draft>();

function readDraft(questionId: string): Draft {
  return drafts.get(questionId) ?? { selections: {}, customText: {} };
}

function writeDraft(questionId: string, draft: Draft) {
  drafts.set(questionId, draft);
}

function clearDraft(questionId: string) {
  drafts.delete(questionId);
}

function outcomeSummary(outcome: ClarificationInboxAsyncOutcome | undefined): string {
  if (!outcome) return "";
  if (outcome.rejected) return outcome.reject_reason ?? "";
  if (outcome.text) return outcome.text;
  return (outcome.answers ?? [])
    .map((answer) =>
      [...(answer.selected_options ?? []), answer.custom_text ?? ""].filter(Boolean).join(", "),
    )
    .filter(Boolean)
    .join(" · ");
}

function useAsyncDraft(questionId: string) {
  const initialDraft = useMemo(() => readDraft(questionId), [questionId]);
  const [draft, setDraft] = useState<Draft>(initialDraft);

  const updateSelection = useCallback(
    (itemId: string, optionId: string) => {
      setDraft((current) => {
        const next: Draft = {
          selections: { ...current.selections, [itemId]: optionId },
          customText: { ...current.customText },
        };
        delete next.customText[itemId];
        writeDraft(questionId, next);
        return next;
      });
    },
    [questionId],
  );

  const updateCustomText = useCallback(
    (itemId: string, value: string) => {
      setDraft((current) => {
        const next: Draft = {
          selections: { ...current.selections },
          customText: { ...current.customText, [itemId]: value },
        };
        if (value.trim() !== "") delete next.selections[itemId];
        writeDraft(questionId, next);
        return next;
      });
    },
    [questionId],
  );

  return { ...draft, updateSelection, updateCustomText };
}

function useAsyncSubmit(question: ClarificationInboxAsyncQuestion, onSettled: () => void) {
  const { t } = useTranslation();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const submissionIdRef = useRef<string>(
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `${question.question_id}-${Date.now()}`,
  );
  const submit = useCallback(
    async (draft: Draft, rejected: boolean) => {
      if (submitting) return;
      const answers: AgentQuestionAnswer[] = [];
      if (!rejected) {
        for (const item of question.questions) {
          const selected = draft.selections[item.id];
          const text = draft.customText[item.id]?.trim();
          if (!selected && !text) {
            setError(t("needsYouInbox:asyncAnswerRequired"));
            return false;
          }
          answers.push({
            question_id: item.id,
            selected_options: selected ? [selected] : [],
            custom_text: text || undefined,
          });
        }
      }
      setSubmitting(true);
      setError(null);
      try {
        const result = await respondToAgentQuestion(question.question_id, {
          answers,
          rejected,
          submission_id: submissionIdRef.current,
        });
        clearDraft(question.question_id);
        if (!result.won) toast(t("needsYouInbox:asyncConcurrentWinner"));
        if (result.won && rejected) toast(t("needsYouInbox:asyncRejected"));
        onSettled();
        return true;
      } catch {
        setError(t("needsYouInbox:asyncSubmitFailed"));
        return false;
      } finally {
        setSubmitting(false);
      }
    },
    [onSettled, question.question_id, question.questions, submitting, t],
  );
  return { submitting, error, submit };
}

function RowChevron({ expanded, pending }: { expanded: boolean; pending: boolean }) {
  if (!pending) return <IconShare className="h-4 w-4 shrink-0 text-muted-foreground" />;
  if (expanded) return <IconChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />;
  return <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />;
}

function AsyncQuestionStatusBadge({ question }: { question: ClarificationInboxAsyncQuestion }) {
  const { t } = useTranslation();
  if (question.status !== "pending") {
    return (
      <span className="text-xs text-muted-foreground" data-testid="async-question-terminal-status">
        {question.status === "withdrawn"
          ? t("needsYouInbox:asyncWithdrawn")
          : t("needsYouInbox:asyncResolved")}
      </span>
    );
  }
  if (question.waiting) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
        <IconClockPause className="h-3.5 w-3.5" />
        {t("needsYouInbox:asyncWaitingForAnswer")}
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
      <IconMessageQuestion className="h-3.5 w-3.5" />
      {t("needsYouInbox:asyncAgentContinues")}
    </span>
  );
}

function AsyncQuestionHeader({
  question,
  expanded,
  provenance,
  onOpen,
  onDismiss,
  dismissDisabled,
}: {
  question: ClarificationInboxAsyncQuestion;
  expanded: boolean;
  provenance: string | null;
  onOpen: () => void;
  onDismiss: () => void;
  dismissDisabled: boolean;
}) {
  const { t } = useTranslation();
  const pending = question.status === "pending";
  const terminalSummary =
    question.status === "withdrawn"
      ? (question.withdraw_reason ?? "")
      : outcomeSummary(question.outcome);
  return (
    <div className="flex items-start gap-3 px-3 py-3">
      <button
        type="button"
        className="min-w-0 flex-1 cursor-pointer text-left"
        onClick={pending ? onOpen : undefined}
        aria-expanded={expanded}
        data-testid="async-question-row-toggle"
      >
        <span className="flex items-center gap-2">
          <RowChevron expanded={expanded} pending={pending} />
          <span className="truncate text-sm font-medium">{asyncQuestionPrimaryText(question)}</span>
        </span>
        <span className="mt-1 flex flex-wrap items-center gap-2 pl-6">
          <AsyncQuestionStatusBadge question={question} />
          {provenance && (
            <span className="text-xs text-muted-foreground" data-testid="async-question-provenance">
              {provenance}
            </span>
          )}
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(question.created_at)}
          </span>
        </span>
        {!pending && terminalSummary && (
          <span className="mt-1 block pl-6 text-xs text-muted-foreground">{terminalSummary}</span>
        )}
      </button>
      {pending && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-11 shrink-0 cursor-pointer"
          disabled={dismissDisabled}
          onClick={onDismiss}
          data-testid="async-question-dismiss"
        >
          {t("needsYouInbox:dismiss")}
        </Button>
      )}
    </div>
  );
}

function AsyncQuestionPhoneSurface({
  question,
  provenance,
  onBack,
  footer,
  children,
}: {
  question: ClarificationInboxAsyncQuestion;
  provenance: string | null;
  onBack: () => void;
  footer: React.ReactNode;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  const surfaceRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    surfaceRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onBack();
        return;
      }
      if (event.key !== "Tab" || !surfaceRef.current) return;
      const focusable = surfaceRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), textarea:not([disabled]), input:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
      previouslyFocused?.focus();
    };
  }, [onBack]);
  return (
    <div
      ref={surfaceRef}
      tabIndex={-1}
      className="fixed inset-0 z-50 flex flex-col bg-background outline-none"
      style={{ height: "100dvh" }}
      role="dialog"
      aria-modal="true"
      aria-label={asyncQuestionPrimaryText(question)}
      data-testid="async-question-answer-surface"
    >
      <div className="flex items-center gap-2 border-b border-border px-3 py-3">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-11 cursor-pointer"
          onClick={onBack}
          data-testid="async-question-back"
        >
          <IconArrowLeft className="mr-1 h-4 w-4" />
          {t("needsYouInbox:asyncBack")}
        </Button>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{question.task_title}</p>
          <p className="truncate text-xs text-muted-foreground">{provenance}</p>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        <AsyncQuestionStatusBadge question={question} />
        {children}
      </div>
      <div className="border-t border-border px-3 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
        {footer}
      </div>
    </div>
  );
}

export function AsyncQuestionRow({
  question,
  onSettled,
}: {
  question: ClarificationInboxAsyncQuestion;
  onSettled: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [expanded, setExpanded] = useState(false);
  const [dismissing, setDismissing] = useState(false);
  const draft = useAsyncDraft(question.question_id);
  const { submitting, error, submit } = useAsyncSubmit(question, onSettled);
  const dismiss = useCallback(async () => {
    if (dismissing) return;
    setDismissing(true);
    try {
      await dismissClarificationInboxBundle(`${ASYNC_SIDECAR_PREFIX}${question.question_id}`);
      onSettled();
    } catch {
      toast.error(t("needsYouInbox:dismissFailed"));
    } finally {
      setDismissing(false);
    }
  }, [dismissing, onSettled, question.question_id, t]);
  const provenance = question.forwarded
    ? t("needsYouInbox:asyncForwardedBy", {
        parent: question.parent_task_title || question.task_title,
      })
    : null;
  const pending = question.status === "pending";
  const runSubmit = async (rejected: boolean) => {
    const settled = await submit(
      { selections: draft.selections, customText: draft.customText },
      rejected,
    );
    if (settled) setExpanded(false);
  };
  const form = (
    <AsyncQuestionForm
      question={question}
      selections={draft.selections}
      customText={draft.customText}
      submitting={submitting}
      error={error}
      onSelect={draft.updateSelection}
      onCustomText={draft.updateCustomText}
      onReject={() => void runSubmit(true)}
      onSubmit={() => void runSubmit(false)}
      includeActions={!isMobile}
    />
  );

  return (
    <div
      className="bg-background"
      data-testid="async-question-row"
      data-question-id={question.question_id}
      data-waiting={question.waiting ? "true" : "false"}
    >
      <AsyncQuestionHeader
        question={question}
        expanded={expanded}
        provenance={provenance}
        onOpen={() => setExpanded(true)}
        onDismiss={() => void dismiss()}
        dismissDisabled={dismissing}
      />
      {expanded && pending && !isMobile && (
        <div className="border-t border-border px-3 pb-3" data-testid="async-question-inline-form">
          {form}
        </div>
      )}
      {expanded && pending && isMobile && (
        <AsyncQuestionPhoneSurface
          question={question}
          provenance={provenance}
          onBack={() => setExpanded(false)}
          footer={
            <AsyncQuestionActions
              submitting={submitting}
              onReject={() => void runSubmit(true)}
              onSubmit={() => void runSubmit(false)}
            />
          }
        >
          {form}
        </AsyncQuestionPhoneSurface>
      )}
    </div>
  );
}
