"use client";

import { useEffect, useState } from "react";
import { IconChevronDown, IconChevronUp, IconMessageQuestion } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { AsyncQuestionRow } from "@/components/needs-you-inbox/async-question-row";
import { useTaskAsyncQuestions } from "@/hooks/domains/session/use-task-async-questions";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

function AsyncQuestionStripError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex shrink-0 items-center justify-between gap-3 border-t border-border px-3 py-2"
      role="alert"
      data-testid="task-async-question-error"
    >
      <span className="text-xs text-destructive">{t("needsYouInbox:chatQuestionsLoadFailed")}</span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 cursor-pointer px-3"
        onClick={onRetry}
      >
        {t("needsYouInbox:chatRetry")}
      </Button>
    </div>
  );
}

export function AsyncQuestionStrip({
  workspaceId,
  taskId,
  hasBlockingClarification,
  enabled = true,
}: {
  workspaceId: string | null;
  taskId: string | null;
  hasBlockingClarification: boolean;
  enabled?: boolean;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const { questions, status, refresh } = useTaskAsyncQuestions(workspaceId, taskId, enabled);
  const isExpanded = expanded && !hasBlockingClarification;

  const getProvenanceText = (question: ClarificationInboxAsyncQuestion) => {
    if (!question.forwarded) return undefined;
    if (question.task_id === taskId) {
      return t("needsYouInbox:asyncFromParent", { parent: question.parent_task_title });
    }
    return t("needsYouInbox:asyncFromChild", { child: question.task_title });
  };

  useEffect(() => {
    if (hasBlockingClarification) setExpanded(false);
  }, [hasBlockingClarification]);

  if (!enabled || !workspaceId || !taskId) return null;
  if (questions.length === 0) {
    if (status === "error") return <AsyncQuestionStripError onRetry={() => void refresh()} />;
    return null;
  }

  const onSettled = () => void refresh();
  return (
    <section
      id="task-async-question-strip"
      className="shrink-0 border-t border-border bg-background"
      data-testid="task-async-question-strip"
      data-expanded={isExpanded ? "true" : "false"}
    >
      <div className="flex min-h-11 items-center gap-2 px-3 py-1">
        <IconMessageQuestion className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground" aria-live="polite">
          {t("needsYouInbox:chatOpenQuestions", { count: questions.length })}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 shrink-0 cursor-pointer px-3"
          aria-expanded={isExpanded}
          disabled={hasBlockingClarification}
          onClick={() => setExpanded((value) => !value)}
          data-testid="task-async-question-toggle"
        >
          {isExpanded ? (
            <>
              {t("needsYouInbox:chatHideQuestions")}
              <IconChevronUp className="ml-1 h-4 w-4" />
            </>
          ) : (
            <>
              {t("needsYouInbox:chatShowQuestions")}
              <IconChevronDown className="ml-1 h-4 w-4" />
            </>
          )}
        </Button>
      </div>
      {status === "error" && <AsyncQuestionStripError onRetry={() => void refresh()} />}
      {isExpanded && (
        <div
          className="max-h-[min(50dvh,28rem)] overflow-y-auto border-t border-border"
          data-testid="task-async-question-list"
        >
          {questions.map((question: ClarificationInboxAsyncQuestion) => (
            <AsyncQuestionRow
              key={question.question_id}
              question={question}
              onSettled={onSettled}
              showDismiss={false}
              provenanceText={getProvenanceText(question)}
            />
          ))}
        </div>
      )}
    </section>
  );
}
