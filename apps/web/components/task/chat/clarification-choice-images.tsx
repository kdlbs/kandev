"use client";

import type { ClarificationRequestMetadata } from "@/lib/types/http";
import { ClarificationMarkdown } from "./clarification-markdown";
import { QuestionImages } from "./question-images";

export function ClarificationChoiceImages({
  metadata,
  sessionId,
}: {
  metadata: ClarificationRequestMetadata;
  sessionId: string;
}) {
  return metadata.question.options
    .filter((option) => option.images?.length)
    .map((option) => (
      <div
        key={option.option_id}
        role="group"
        aria-label={option.label}
        className="mt-2 border-l-2 border-border pl-3"
      >
        <ClarificationMarkdown variant="inline" className="text-xs font-medium">
          {option.label}
        </ClarificationMarkdown>
        <QuestionImages
          pendingId={metadata.pending_id}
          questionId={metadata.question_id ?? metadata.question.id}
          sessionId={sessionId}
          images={option.images}
          historical
        />
      </div>
    ));
}
