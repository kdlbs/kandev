"use client";

import { IconCheck, IconClockPause, IconMessageQuestion, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAgentQuestionDetail } from "@/hooks/domains/session/use-agent-question-detail";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";
import type { ClarificationQuestion, Message } from "@/lib/types/http";
import { ClarificationMarkdown } from "../clarification-markdown";

function QuestionOutcome({
  question,
  item,
}: {
  question: ClarificationInboxAsyncQuestion;
  item: ClarificationQuestion;
}) {
  const answer = question.outcome?.answers?.find((entry) => entry.question_id === item.id);
  const labels = (answer?.selected_options ?? [])
    .map((optionId) => item.options.find((option) => option.option_id === optionId)?.label)
    .filter((label): label is string => Boolean(label));
  const details = [...labels, answer?.custom_text, question.outcome?.text].filter(Boolean);

  if (details.length === 0) return null;
  return <p className="mt-1 text-xs text-foreground/80">{details.join(", ")}</p>;
}

function QuestionStatus({ question }: { question: ClarificationInboxAsyncQuestion }) {
  const { t } = useTranslation();
  switch (question.status) {
    case "pending":
      return question.waiting ? (
        <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
          <IconClockPause className="h-3.5 w-3.5" />
          {t("needsYouInbox:asyncWaitingForAnswer")}
        </span>
      ) : (
        <span className="text-xs text-muted-foreground">
          {t("needsYouInbox:asyncAgentContinues")}
        </span>
      );
    case "answered":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-foreground/80">
          <IconCheck className="h-3.5 w-3.5 text-green-500" />
          {t("needsYouInbox:asyncAnswered")}
        </span>
      );
    case "rejected":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
          <IconX className="h-3.5 w-3.5" />
          {t("needsYouInbox:asyncRejectedStatus")}
        </span>
      );
    case "withdrawn":
      return (
        <span className="text-xs text-muted-foreground">{t("needsYouInbox:asyncWithdrawn")}</span>
      );
    case "stale":
      return <span className="text-xs text-muted-foreground">{t("needsYouInbox:asyncStale")}</span>;
  }
}

function QuestionRequestBody({ question }: { question: ClarificationInboxAsyncQuestion }) {
  return (
    <div className="mt-2 space-y-2">
      {question.questions.map((item) => (
        <section key={item.id}>
          {item.title && <p className="text-xs font-medium text-foreground">{item.title}</p>}
          <ClarificationMarkdown variant="block" className="text-xs text-muted-foreground">
            {item.prompt}
          </ClarificationMarkdown>
          {question.status === "answered" && <QuestionOutcome question={question} item={item} />}
        </section>
      ))}
      {question.status === "rejected" && question.outcome?.reject_reason && (
        <p className="text-xs text-muted-foreground">{question.outcome.reject_reason}</p>
      )}
      {question.status === "withdrawn" && question.withdraw_reason && (
        <p className="text-xs text-muted-foreground">{question.withdraw_reason}</p>
      )}
      {question.status === "stale" && question.withdraw_reason && (
        <p className="text-xs text-muted-foreground">{question.withdraw_reason}</p>
      )}
    </div>
  );
}

function metadataQuestionId(message: Message): string | null {
  const value = message.metadata?.async_question_id;
  return typeof value === "string" && value.length > 0 ? value : null;
}

export function AgentQuestionMessage({ comment }: { comment: Message }) {
  const { t } = useTranslation();
  const questionId = metadataQuestionId(comment);
  const { question, status, refresh } = useAgentQuestionDetail(questionId);

  return (
    <article
      className="w-full"
      data-testid="agent-question-message"
      data-question-id={questionId ?? ""}
    >
      <div className="flex items-start gap-3">
        <IconMessageQuestion className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-xs font-medium text-foreground/80">
              {t("needsYouInbox:asyncQuestionTranscriptTitle")}
            </span>
            {question && <QuestionStatus question={question} />}
            {!question && status === "loading" && (
              <span className="text-xs text-muted-foreground" role="status">
                {t("needsYouInbox:asyncQuestionDetailsLoading")}
              </span>
            )}
          </div>
          {question && <QuestionRequestBody question={question} />}
          {!question && status === "error" && (
            <div className="mt-1 flex items-center gap-2">
              <span className="text-xs text-muted-foreground">
                {t("needsYouInbox:asyncQuestionDetailsUnavailable")}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="min-h-11 cursor-pointer px-3 md:min-h-7"
                onClick={() => void refresh()}
              >
                {t("needsYouInbox:chatRetry")}
              </Button>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}
