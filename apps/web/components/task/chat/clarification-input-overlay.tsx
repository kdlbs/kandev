"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  IconX,
  IconMessageQuestion,
  IconInfoCircle,
  IconCornerDownLeft,
  IconCheck,
} from "@tabler/icons-react";
import ReactMarkdown from "react-markdown";
import { cn } from "@/lib/utils";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";
import type {
  Message,
  ClarificationRequestMetadata,
  ClarificationAnswer,
  ClarificationQuestion,
  ClarificationOption,
} from "@/lib/types/http";
import { useClarificationGroup } from "@/hooks/domains/session/use-clarification-group";

type ClarificationInputOverlayProps = {
  // Single message kept for legacy callers; the new multi-question UX uses
  // `messages` to render every pending question in the bundle stacked together.
  message?: Message | null;
  messages?: readonly Message[] | null;
  onResolved: () => void;
};

type SingleQuestionMeta = {
  message: Message;
  metadata: ClarificationRequestMetadata;
  question: ClarificationQuestion;
  questionId: string;
};

function readSingleQuestionMeta(message: Message | null | undefined): SingleQuestionMeta | null {
  if (!message) return null;
  const metadata = message.metadata as ClarificationRequestMetadata | undefined;
  if (!metadata?.question) return null;
  const questionId = metadata.question_id ?? metadata.question.id;
  if (!questionId) return null;
  return { message, metadata, question: metadata.question, questionId };
}

type QuestionCardProps = {
  index: number;
  total: number;
  meta: SingleQuestionMeta;
  selectedOption: string | null;
  customDraft: string;
  onCustomDraftChange: (value: string) => void;
  onSubmitOption: (optionId: string) => void;
  onSubmitCustom: (text: string) => void;
  isSubmitting: boolean;
  showAgentDisconnected: boolean;
  isCommitted: boolean;
  customCommittedText: string | null;
};

const KEY_HINT_LIMIT = 9;

function ClarificationOptions({
  options,
  isSubmitting,
  onSelectOption,
  selectedOption,
  customCommittedText,
  showKeyHints,
}: {
  options: ClarificationOption[];
  isSubmitting: boolean;
  onSelectOption: (optionId: string) => void;
  selectedOption: string | null;
  customCommittedText: string | null;
  showKeyHints: boolean;
}) {
  return (
    <div className="space-y-1 mt-1.5">
      {options.map((option, idx) => {
        const isSelected = selectedOption === option.option_id;
        const dimmed = customCommittedText !== null;
        return (
          <button
            key={option.option_id}
            type="button"
            onClick={() => onSelectOption(option.option_id)}
            disabled={isSubmitting}
            data-testid="clarification-option"
            data-selected={isSelected ? "true" : "false"}
            className={cn(
              "group flex items-start gap-2.5 w-full text-left text-sm rounded-md px-2 py-1.5 transition-colors border",
              isSelected
                ? "bg-blue-500/15 border-blue-500/40 text-foreground"
                : "border-transparent hover:bg-blue-500/10 hover:border-blue-500/20 text-foreground/85",
              isSubmitting ? "opacity-60 cursor-not-allowed" : "cursor-pointer",
              dimmed && !isSelected ? "opacity-60" : "",
            )}
          >
            {showKeyHints && idx < KEY_HINT_LIMIT ? (
              <kbd
                aria-hidden="true"
                className="select-none font-mono text-[10px] px-1.5 py-0.5 rounded border border-border bg-muted text-muted-foreground leading-none mt-0.5"
              >
                {idx + 1}
              </kbd>
            ) : (
              <span className="text-muted-foreground/70 text-sm leading-5">•</span>
            )}
            <span className="flex-1 min-w-0">
              <span
                data-testid="clarification-option-label"
                className="block leading-5 font-medium"
              >
                {option.label}
              </span>
              {option.description && (
                <span
                  data-testid="clarification-option-description"
                  className="block text-muted-foreground/80 mt-0.5 text-xs leading-snug"
                >
                  {option.description}
                </span>
              )}
            </span>
            {isSelected && <IconCheck className="h-3.5 w-3.5 text-blue-500 mt-1 flex-shrink-0" />}
          </button>
        );
      })}
    </div>
  );
}

function ClarificationCustomInput({
  draft,
  isSubmitting,
  onChange,
  onSubmit,
  committedText,
}: {
  draft: string;
  isSubmitting: boolean;
  onChange: (text: string) => void;
  onSubmit: (text: string) => void;
  committedText: string | null;
}) {
  return (
    <div className="mt-2 flex items-center gap-2 px-2 py-1.5 rounded-md border border-dashed border-border/70 bg-muted/30">
      <span className="text-muted-foreground text-xs">↳</span>
      <input
        type="text"
        placeholder={
          committedText !== null ? "Press Enter to update your answer…" : "Or type a custom answer…"
        }
        value={draft}
        onChange={(e) => onChange(e.target.value)}
        disabled={isSubmitting}
        data-testid="clarification-input"
        className="flex-1 text-sm bg-transparent placeholder:text-muted-foreground/60 focus:outline-none"
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey && draft.trim()) {
            e.preventDefault();
            onSubmit(draft.trim());
          }
        }}
      />
      <kbd
        aria-hidden="true"
        className="select-none flex items-center gap-1 font-mono text-[10px] px-1.5 py-0.5 rounded border border-border bg-background text-muted-foreground"
      >
        <IconCornerDownLeft className="h-2.5 w-2.5" />
        Enter
      </kbd>
    </div>
  );
}

function ClarificationCard({
  index,
  total,
  meta,
  selectedOption,
  customDraft,
  onCustomDraftChange,
  onSubmitOption,
  onSubmitCustom,
  isSubmitting,
  showAgentDisconnected,
  isCommitted,
  customCommittedText,
}: QuestionCardProps) {
  const { question, metadata } = meta;
  const showProgress = total > 1;
  const showKeyHints = total === 1; // Only the single-question case can rely on top-level digit shortcuts.

  return (
    <div
      data-testid="clarification-question-card"
      data-question-id={meta.questionId}
      data-answered={isCommitted ? "true" : "false"}
      className={cn(
        "flex gap-3 px-3 py-2",
        showProgress ? "border-l-2" : "",
        isCommitted ? "border-l-blue-500/60" : "border-l-transparent",
      )}
    >
      <div className="flex-shrink-0 mt-0.5">
        <IconMessageQuestion
          className={cn("h-4 w-4", isCommitted ? "text-blue-500" : "text-blue-500/80")}
        />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          {showProgress && (
            <span
              data-testid="clarification-progress-chip"
              className="select-none text-[10px] uppercase tracking-wider font-semibold rounded-full bg-blue-500/15 text-blue-600 dark:text-blue-300 px-2 py-0.5"
            >
              Question {index + 1} of {total}
            </span>
          )}
          {metadata.question.title && (
            <span className="text-xs text-muted-foreground/80">{metadata.question.title}</span>
          )}
          {isCommitted && (
            <span
              data-testid="clarification-question-answered"
              className="ml-auto inline-flex items-center gap-1 text-[10px] text-blue-500"
            >
              <IconCheck className="h-3 w-3" />
              Answered
            </span>
          )}
        </div>
        <div className="markdown-body max-w-none text-sm [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
          <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
            {question.prompt}
          </ReactMarkdown>
        </div>
        <ClarificationOptions
          options={question.options}
          isSubmitting={isSubmitting}
          onSelectOption={onSubmitOption}
          selectedOption={selectedOption}
          customCommittedText={customCommittedText}
          showKeyHints={showKeyHints}
        />
        {showAgentDisconnected && (
          <div
            data-testid="clarification-deferred-notice"
            className="mt-2 text-xs text-amber-500/90 flex items-center gap-1.5"
          >
            <IconInfoCircle className="h-3.5 w-3.5 flex-shrink-0" />
            The agent has moved on. Your response will be sent as a new message.
          </div>
        )}
        <ClarificationCustomInput
          draft={customDraft}
          isSubmitting={isSubmitting}
          onChange={onCustomDraftChange}
          onSubmit={onSubmitCustom}
          committedText={customCommittedText}
        />
      </div>
    </div>
  );
}

// resolveQuestionMessages picks the array of messages that should be rendered.
// New callers pass `messages` directly; legacy callers pass a single `message`
// and we adapt that into a one-element array so the same component handles both.
function resolveQuestionMessages(props: ClarificationInputOverlayProps): Message[] {
  if (props.messages && props.messages.length > 0) return [...props.messages];
  if (props.message) return [props.message];
  return [];
}

function sortMessagesByQuestionIndex(messages: Message[]): Message[] {
  return messages.slice().sort((a, b) => {
    const ai = (a.metadata as ClarificationRequestMetadata | undefined)?.question_index ?? 0;
    const bi = (b.metadata as ClarificationRequestMetadata | undefined)?.question_index ?? 0;
    return ai - bi;
  });
}

function useResolveCallback(
  submitState: ReturnType<typeof useClarificationGroup>["submitState"],
  onResolved: () => void,
) {
  const last = useRef(submitState);
  useEffect(() => {
    if (last.current !== submitState && (submitState === "ok" || submitState === "expired")) {
      onResolved();
    }
    last.current = submitState;
  }, [submitState, onResolved]);
}

function useSingleQuestionShortcuts(
  meta: SingleQuestionMeta | null,
  group: ReturnType<typeof useClarificationGroup>,
) {
  useEffect(() => {
    if (!meta) return;
    const onKey = (e: KeyboardEvent) => {
      if (
        e.target instanceof HTMLElement &&
        (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA")
      ) {
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        void group.skipAll("User skipped");
        return;
      }
      const num = Number.parseInt(e.key, 10);
      if (!Number.isFinite(num) || num < 1) return;
      const opt = meta.question.options[num - 1];
      if (!opt) return;
      e.preventDefault();
      void group.recordAnswer(meta.questionId, {
        question_id: meta.questionId,
        selected_options: [opt.option_id],
      });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [meta, group]);
}

type ClarificationCardListProps = {
  messages: Message[];
  group: ReturnType<typeof useClarificationGroup>;
  customDrafts: Record<string, string>;
  setCustomDrafts: React.Dispatch<React.SetStateAction<Record<string, string>>>;
};

function ClarificationCardList({
  messages,
  group,
  customDrafts,
  setCustomDrafts,
}: ClarificationCardListProps) {
  const showAgentDisconnectedAtTop = messages.some(
    (m) => (m.metadata as ClarificationRequestMetadata | undefined)?.agent_disconnected === true,
  );
  const isSubmitting = group.submitState === "submitting";

  return (
    <div className="divide-y divide-border/40">
      {messages.map((m, idx) => {
        const meta = readSingleQuestionMeta(m);
        if (!meta) return null;
        const stored = group.answers[meta.questionId];
        const selectedOption =
          stored?.selected_options && stored.selected_options.length > 0
            ? stored.selected_options[0]
            : null;
        const customCommittedText = stored?.custom_text ?? null;
        const isCommitted = Boolean(stored);
        const draft = customDrafts[meta.questionId] ?? "";

        return (
          <ClarificationCard
            key={m.id}
            index={idx}
            total={messages.length}
            meta={meta}
            selectedOption={selectedOption}
            customDraft={draft}
            onCustomDraftChange={(value) =>
              setCustomDrafts((prev) => ({ ...prev, [meta.questionId]: value }))
            }
            onSubmitOption={(optionId) =>
              void group.recordAnswer(meta.questionId, {
                question_id: meta.questionId,
                selected_options: [optionId],
              })
            }
            onSubmitCustom={(text) => {
              if (!text.trim()) return;
              void group.recordAnswer(meta.questionId, {
                question_id: meta.questionId,
                selected_options: [],
                custom_text: text.trim(),
              });
            }}
            isSubmitting={isSubmitting}
            showAgentDisconnected={idx === 0 ? showAgentDisconnectedAtTop : false}
            isCommitted={isCommitted}
            customCommittedText={customCommittedText}
          />
        );
      })}
    </div>
  );
}

export function ClarificationInputOverlay(props: ClarificationInputOverlayProps) {
  const { onResolved } = props;
  const sortedMessages = useMemo(
    () => sortMessagesByQuestionIndex(resolveQuestionMessages(props)),
    [props],
  );
  const group = useClarificationGroup(sortedMessages);
  const [customDrafts, setCustomDrafts] = useState<Record<string, string>>({});

  useResolveCallback(group.submitState, onResolved);
  const singleMeta = sortedMessages.length === 1 ? readSingleQuestionMeta(sortedMessages[0]) : null;
  useSingleQuestionShortcuts(singleMeta, group);

  if (sortedMessages.length === 0) return null;
  const isSubmitting = group.submitState === "submitting";

  return (
    <div className="relative" data-testid="clarification-overlay">
      <button
        type="button"
        onClick={() => void group.skipAll("User skipped")}
        disabled={isSubmitting}
        className="absolute top-2 right-3 text-muted-foreground hover:text-foreground z-10 cursor-pointer disabled:opacity-50"
        data-testid="clarification-skip"
        aria-label="Skip all questions"
      >
        <IconX className="h-4 w-4" />
      </button>
      {group.total > 1 && (
        <div className="px-3 pt-2 pb-1 flex items-center gap-2">
          <span
            data-testid="clarification-group-progress"
            className="text-xs text-muted-foreground"
          >
            {group.answeredCount} of {group.total} answered
          </span>
          {group.answeredCount < group.total && (
            <span className="text-xs text-muted-foreground/70">· all required to continue</span>
          )}
        </div>
      )}
      <ClarificationCardList
        messages={sortedMessages}
        group={group}
        customDrafts={customDrafts}
        setCustomDrafts={setCustomDrafts}
      />
    </div>
  );
}

// Placeholder export to keep the type in this file consumable by tests that
// previously imported a single answer literal — keeps the legacy path
// expressive without any runtime cost.
export type { ClarificationAnswer };
