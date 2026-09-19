"use client";

import { IconMessageQuestion, IconCheck, IconX } from "@tabler/icons-react";
import type {
  ClarificationAnswer,
  ClarificationQuestion,
  Message,
  ClarificationRequestMetadata,
} from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { ClarificationMarkdown } from "../clarification-markdown";

type ClarificationRequestMessageProps = {
  comment: Message;
};

function AnswerSummary({
  question,
  response,
}: {
  question: ClarificationQuestion;
  response: ClarificationAnswer | undefined;
}) {
  const { t } = useTranslation();
  if (!response) return <span>{t("task:clarificationNoSelection")}</span>;

  const selectedOptions = (response.selected_options ?? [])
    .map((optionId) => question.options.find((option) => option.option_id === optionId))
    .filter((option) => option !== undefined);
  const hasCustomText = Boolean(response.custom_text);

  if (selectedOptions.length === 0 && !hasCustomText) {
    return <span>{t("task:clarificationNoSelection")}</span>;
  }

  return (
    <span className="min-w-0">
      {selectedOptions.map((option, index) => (
        <span key={option.option_id}>
          {index > 0 ? ", " : null}
          <ClarificationMarkdown variant="inline">{option.label}</ClarificationMarkdown>
        </span>
      ))}
      {response.custom_text && (
        <span className="whitespace-pre-wrap">
          {selectedOptions.length > 0 ? ", " : null}
          {`"${response.custom_text}"`}
        </span>
      )}
    </span>
  );
}

/**
 * Displays a resolved or superseded clarification request in the chat history.
 * The active pending clarification is shown in the input area instead.
 */
export function ClarificationRequestMessage({ comment }: ClarificationRequestMessageProps) {
  const { t } = useTranslation();
  const metadata = comment.metadata as ClarificationRequestMetadata | undefined;

  if (!metadata?.question) {
    return null;
  }

  const question = metadata.question;
  const status = metadata.status;
  const isAnswered = status === "answered";
  const isSkipped = status === "rejected";
  const isExpired = status === "expired";
  const isCancelled = status === "cancelled";

  const getStatusIndicator = () => {
    if (isAnswered) {
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    }
    if (isSkipped || isCancelled) {
      return <IconX className="h-3.5 w-3.5 text-muted-foreground" />;
    }
    if (isExpired) {
      return <IconX className="h-3.5 w-3.5 text-orange-500" />;
    }
    return null;
  };

  return (
    <div className="w-full" data-testid="clarification-request-message">
      <div className="flex items-start gap-3 w-full">
        {/* Icon */}
        <div className="flex-shrink-0 mt-0.5">
          <IconMessageQuestion className="h-4 w-4 text-muted-foreground" />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {/* Question */}
          {question.title && (
            <ClarificationMarkdown
              variant="inline"
              className="mb-0.5 text-xs font-medium text-foreground/80"
            >
              {question.title}
            </ClarificationMarkdown>
          )}
          <ClarificationMarkdown
            variant="block"
            className="max-w-none text-xs text-muted-foreground [&>*:first-child]:mt-0 [&>*:last-child]:mb-0"
          >
            {question.prompt}
          </ClarificationMarkdown>

          {/* Answer - indented below question */}
          {isAnswered && (
            <div className="mt-1 ml-3 flex items-start gap-1.5 text-xs text-foreground/80">
              {getStatusIndicator()}
              <AnswerSummary question={question} response={metadata.response} />
              {metadata.agent_disconnected && (
                <span className="text-muted-foreground">{t("task:sentAsNewMessage")}</span>
              )}
            </div>
          )}
          {isSkipped && (
            <div className="mt-1 ml-3 flex items-center gap-1.5 text-xs text-muted-foreground">
              {getStatusIndicator()}
              {t("task:skipped")}
            </div>
          )}
          {isCancelled && (
            <div className="mt-1 ml-3 flex items-center gap-1.5 text-xs text-muted-foreground">
              {getStatusIndicator()}
              {t("task:cancelled")}
            </div>
          )}
          {isExpired && (
            <div
              data-testid="clarification-expired-notice"
              className="mt-1 ml-3 flex items-center gap-1.5 text-xs text-orange-500/80"
            >
              {getStatusIndicator()}
              {t("task:timedOutAgentMovedOn")}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
