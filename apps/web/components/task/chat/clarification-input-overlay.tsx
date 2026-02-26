"use client";

import { useCallback, useState } from "react";
import { IconX, IconMessageQuestion } from "@tabler/icons-react";
import ReactMarkdown from "react-markdown";
import { cn } from "@/lib/utils";
import { getBackendConfig } from "@/lib/config";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";
import type {
  Message,
  ClarificationRequestMetadata,
  ClarificationAnswer,
  ClarificationQuestion,
} from "@/lib/types/http";

const RESULT_EXPIRED = "expired";

type ClarificationInputOverlayProps = {
  message: Message;
  onResolved: () => void;
};

/**
 * Post a response to the clarification endpoint. Returns true on success.
 */
async function postClarificationResponse(
  pendingId: string,
  body: Record<string, unknown>,
): Promise<"ok" | "expired" | "error"> {
  const { apiBaseUrl } = getBackendConfig();
  const response = await fetch(`${apiBaseUrl}/api/v1/clarification/${pendingId}/respond`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (response.ok) return "ok";
  if (response.status === 410) {
    const data = await response.json().catch(() => ({}));
    console.warn("Clarification expired:", data);
    return RESULT_EXPIRED;
  }
  console.error("Clarification request failed:", response.status, response.statusText);
  return "error";
}

function handleClarificationResult(result: "ok" | "expired" | "error", onResolved: () => void) {
  if (result === "ok" || result === RESULT_EXPIRED) onResolved();
}

type ClarificationOptionsProps = {
  question: ClarificationQuestion;
  isSubmitting: boolean;
  onSelectOption: (optionId: string) => void;
};

function ClarificationOptions({
  question,
  isSubmitting,
  onSelectOption,
}: ClarificationOptionsProps) {
  return (
    <div className="space-y-0.5 mb-1.5 ml-6">
      {question.options.map((option) => (
        <button
          key={option.option_id}
          type="button"
          onClick={() => onSelectOption(option.option_id)}
          disabled={isSubmitting}
          className={cn(
            "flex items-start gap-2 w-full text-left text-xs rounded px-1.5 py-0.5 -ml-1.5 transition-colors",
            "hover:bg-blue-500/15 hover:text-blue-600 dark:hover:text-blue-400",
            isSubmitting ? "opacity-50 cursor-not-allowed" : "text-foreground/80 cursor-pointer",
          )}
        >
          <span className="text-muted-foreground flex-shrink-0">•</span>
          <span>{option.label}</span>
          {option.description && (
            <span className="text-muted-foreground/60">— {option.description}</span>
          )}
        </button>
      ))}
    </div>
  );
}

type ClarificationCustomInputProps = {
  customText: string;
  isSubmitting: boolean;
  onChange: (text: string) => void;
  onSubmit: () => void;
};

function ClarificationCustomInput({
  customText,
  isSubmitting,
  onChange,
  onSubmit,
}: ClarificationCustomInputProps) {
  return (
    <div className="flex items-center gap-2 ml-6">
      <span className="text-muted-foreground flex-shrink-0">•</span>
      <input
        type="text"
        placeholder="Type something..."
        value={customText}
        onChange={(e) => onChange(e.target.value)}
        disabled={isSubmitting}
        className="flex-1 text-sm bg-transparent placeholder:text-muted-foreground focus:outline-none"
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey && customText.trim()) {
            e.preventDefault();
            onSubmit();
          }
        }}
      />
    </div>
  );
}

type UseClarificationHandlersParams = {
  metadata: ClarificationRequestMetadata | undefined;
  isSubmitting: boolean;
  setIsSubmitting: React.Dispatch<React.SetStateAction<boolean>>;
  customText: string;
  onResolved: () => void;
};

function useClarificationHandlers({
  metadata,
  isSubmitting,
  setIsSubmitting,
  customText,
  onResolved,
}: UseClarificationHandlersParams) {
  const handleSubmitOption = useCallback(
    async (optionId: string) => {
      if (!metadata?.pending_id || isSubmitting) return;
      setIsSubmitting(true);
      try {
        const answer: ClarificationAnswer = {
          question_id: metadata.question.id,
          selected_options: [optionId],
        };
        const result = await postClarificationResponse(metadata.pending_id, {
          answers: [answer],
          rejected: false,
        });
        handleClarificationResult(result, onResolved);
      } catch (error) {
        console.error("Failed to submit clarification response:", error);
      } finally {
        setIsSubmitting(false);
      }
    },
    [metadata, isSubmitting, onResolved, setIsSubmitting],
  );

  const handleSubmitCustom = useCallback(async () => {
    if (!metadata?.pending_id || isSubmitting || !customText.trim()) return;
    setIsSubmitting(true);
    try {
      const answer: ClarificationAnswer = {
        question_id: metadata.question.id,
        selected_options: [],
        custom_text: customText.trim(),
      };
      const result = await postClarificationResponse(metadata.pending_id, {
        answers: [answer],
        rejected: false,
      });
      handleClarificationResult(result, onResolved);
    } catch (error) {
      console.error("Failed to submit clarification response:", error);
    } finally {
      setIsSubmitting(false);
    }
  }, [metadata, isSubmitting, customText, onResolved, setIsSubmitting]);

  const handleSkip = useCallback(async () => {
    if (!metadata?.pending_id || isSubmitting) return;
    setIsSubmitting(true);
    try {
      const result = await postClarificationResponse(metadata.pending_id, {
        answers: [],
        rejected: true,
        reject_reason: "User skipped",
      });
      handleClarificationResult(result, onResolved);
    } catch (error) {
      console.error("Failed to skip clarification:", error);
    } finally {
      setIsSubmitting(false);
    }
  }, [metadata, isSubmitting, onResolved, setIsSubmitting]);

  return { handleSubmitOption, handleSubmitCustom, handleSkip };
}

/**
 * Inline clarification UI - simple numbered text options like Conductor.
 */
export function ClarificationInputOverlay({ message, onResolved }: ClarificationInputOverlayProps) {
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  const [customText, setCustomText] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const { handleSubmitOption, handleSubmitCustom, handleSkip } = useClarificationHandlers({
    metadata,
    isSubmitting,
    setIsSubmitting,
    customText,
    onResolved,
  });

  if (!metadata?.question) return null;

  const question = metadata.question;

  return (
    <div className="relative px-3 py-2">
      <button
        type="button"
        onClick={handleSkip}
        disabled={isSubmitting}
        className="absolute top-2 right-3 text-muted-foreground hover:text-foreground z-10"
      >
        <IconX className="h-4 w-4" />
      </button>

      <div className="pr-6">
        <div className="flex items-start gap-2 mb-1">
          <IconMessageQuestion className="h-4 w-4 text-blue-500 flex-shrink-0 mt-0.5" />
          <div className="prose prose-sm dark:prose-invert max-w-none text-sm prose-p:my-1 prose-p:leading-relaxed prose-ul:my-1 prose-ol:my-1 prose-li:my-0.5 prose-pre:my-2 prose-headings:text-foreground prose-headings:font-bold prose-strong:text-foreground [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
            <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
              {question.prompt}
            </ReactMarkdown>
          </div>
        </div>
        <ClarificationOptions
          question={question}
          isSubmitting={isSubmitting}
          onSelectOption={handleSubmitOption}
        />
        <ClarificationCustomInput
          customText={customText}
          isSubmitting={isSubmitting}
          onChange={setCustomText}
          onSubmit={handleSubmitCustom}
        />
      </div>
    </div>
  );
}
