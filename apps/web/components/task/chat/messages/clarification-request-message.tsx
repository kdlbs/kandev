'use client';

import { IconMessageQuestion, IconCheck, IconX } from '@tabler/icons-react';
import type { Message, ClarificationRequestMetadata } from '@/lib/types/http';

type ClarificationRequestMessageProps = {
  comment: Message;
};

/**
 * Displays a resolved clarification request in the chat history.
 * Pending clarifications are shown in the input area instead.
 */
export function ClarificationRequestMessage({ comment }: ClarificationRequestMessageProps) {
  const metadata = comment.metadata as ClarificationRequestMetadata | undefined;

  if (!metadata?.question) {
    return null;
  }

  const question = metadata.question;
  const status = metadata.status;
  const isAnswered = status === 'answered';
  const isSkipped = status === 'rejected';

  const getStatusIndicator = () => {
    if (isAnswered) {
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    }
    if (isSkipped) {
      return <IconX className="h-3.5 w-3.5 text-muted-foreground" />;
    }
    return null;
  };

  // Get the answer summary for display
  const getAnswerSummary = () => {
    const response = metadata.response;
    if (!response) return 'No selection';

    const parts: string[] = [];

    // Get selected option labels
    if (response.selected_options?.length) {
      for (const optionId of response.selected_options) {
        const option = question.options.find((o) => o.option_id === optionId);
        if (option) {
          parts.push(option.label);
        }
      }
    }

    // Add custom text
    if (response.custom_text) {
      parts.push(`"${response.custom_text}"`);
    }

    return parts.length > 0 ? parts.join(', ') : 'No selection';
  };

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full">
        {/* Icon */}
        <div className="flex-shrink-0 mt-0.5">
          <IconMessageQuestion className="h-4 w-4 text-muted-foreground" />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {/* Question */}
          <div className="text-xs font-medium text-muted-foreground">
            {question.prompt}
          </div>

          {/* Answer - indented below question */}
          {isAnswered && (
            <div className="mt-1 ml-3 flex items-center gap-1.5 text-xs text-foreground/80">
              {getStatusIndicator()}
              {getAnswerSummary()}
            </div>
          )}
          {isSkipped && (
            <div className="mt-1 ml-3 flex items-center gap-1.5 text-xs text-muted-foreground">
              {getStatusIndicator()}
              Skipped
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
