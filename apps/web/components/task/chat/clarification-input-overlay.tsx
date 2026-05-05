"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconX, IconMessageQuestion, IconInfoCircle } from "@tabler/icons-react";
import ReactMarkdown from "react-markdown";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";
import type {
  Message,
  ClarificationRequestMetadata,
  ClarificationAnswer,
  ClarificationQuestion,
} from "@/lib/types/http";
import { useClarificationGroup } from "@/hooks/domains/session/use-clarification-group";
import {
  ClarificationCarouselNav,
  ClarificationCustomInput,
  ClarificationOptions,
  ClarificationStepper,
} from "./clarification-overlay-parts";

type ClarificationInputOverlayProps = {
  messages: readonly Message[] | null | undefined;
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

function resolveQuestionMessages(messages: readonly Message[] | null | undefined): Message[] {
  if (messages && messages.length > 0) return [...messages];
  return [];
}

function sortMessagesByQuestionIndex(messages: Message[]): Message[] {
  return messages.slice().sort((a, b) => {
    const ai = (a.metadata as ClarificationRequestMetadata | undefined)?.question_index ?? 0;
    const bi = (b.metadata as ClarificationRequestMetadata | undefined)?.question_index ?? 0;
    return ai - bi;
  });
}

type CardProps = {
  meta: SingleQuestionMeta;
  index: number;
  total: number;
  selectedOption: string | null;
  customCommittedText: string | null;
  customDraft: string;
  isSubmitting: boolean;
  showAgentDisconnected: boolean;
  onSelectOption: (optionId: string) => void;
  onCustomDraftChange: (text: string) => void;
  onSubmitCustom: (text: string) => void;
};

function ClarificationCard(props: CardProps) {
  const {
    meta,
    index,
    total,
    selectedOption,
    customCommittedText,
    customDraft,
    isSubmitting,
    showAgentDisconnected,
    onSelectOption,
    onCustomDraftChange,
    onSubmitCustom,
  } = props;
  const { question, metadata } = meta;
  return (
    <div
      data-testid="clarification-question-card"
      data-question-id={meta.questionId}
      data-question-index={String(index)}
      className="px-4 pt-3 pb-4"
    >
      <div className="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
        <IconMessageQuestion className="h-4 w-4 text-blue-500" />
        {total > 1 && (
          <span data-testid="clarification-progress-chip">
            Question {index + 1} of {total}
          </span>
        )}
        {metadata.question.title && (
          <span className="text-muted-foreground/70">
            {total > 1 ? "· " : ""}
            {metadata.question.title}
          </span>
        )}
      </div>
      <div className="markdown-body max-w-none text-sm font-medium [&>*:first-child]:mt-0 [&>*:last-child]:mb-0 mb-3">
        <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
          {question.prompt}
        </ReactMarkdown>
      </div>
      <ClarificationOptions
        options={question.options}
        selectedOption={selectedOption}
        isSubmitting={isSubmitting}
        onSelectOption={onSelectOption}
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
        committedText={customCommittedText}
        onChange={onCustomDraftChange}
        onSubmit={onSubmitCustom}
      />
    </div>
  );
}

function useResolveCallback(
  submitState: ReturnType<typeof useClarificationGroup>["submitState"],
  onResolved: () => void,
) {
  const last = useRef(submitState);
  useEffect(() => {
    if (last.current !== submitState && submitState === "ok") {
      onResolved();
    }
    last.current = submitState;
  }, [submitState, onResolved]);
}

type CarouselShortcutArgs = {
  meta: SingleQuestionMeta;
  activeIndex: number;
  total: number;
  canSubmit: boolean;
  onPick: (index: number) => void;
  onPrev: () => void;
  onNext: () => void;
  onSkip: () => void;
  onSubmit: () => void;
};

// shouldIgnoreShortcut filters out events that the overlay must not handle:
// keystrokes inside an input/textarea (the user is typing) and any modifier
// combo (so we don't hijack browser shortcuts like Cmd/Ctrl+1..9 for tab
// switching or Alt+ArrowLeft for back-navigation).
function shouldIgnoreShortcut(e: KeyboardEvent): boolean {
  if (
    e.target instanceof HTMLElement &&
    (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA")
  ) {
    return true;
  }
  return e.metaKey || e.ctrlKey || e.altKey || e.shiftKey;
}

function CarouselKeyboardShortcuts(args: CarouselShortcutArgs) {
  const optionsCount = args.meta.question.options.length;
  const isLast = args.activeIndex === args.total - 1;
  const { canSubmit, onPick, onPrev, onNext, onSkip, onSubmit } = args;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (shouldIgnoreShortcut(e)) return;
      if (e.key === "Escape") {
        e.preventDefault();
        onSkip();
        return;
      }
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        onPrev();
        return;
      }
      if (e.key === "ArrowRight") {
        e.preventDefault();
        if (isLast && canSubmit) onSubmit();
        else onNext();
        return;
      }
      const num = Number.parseInt(e.key, 10);
      if (Number.isFinite(num) && num >= 1 && num <= optionsCount) {
        e.preventDefault();
        onPick(num - 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [optionsCount, isLast, canSubmit, onPick, onPrev, onNext, onSkip, onSubmit]);
  return null;
}

type CarouselBodyProps = {
  sortedMessages: Message[];
  group: ReturnType<typeof useClarificationGroup>;
  activeIndex: number;
  setActiveIndex: (idx: number) => void;
  customDrafts: Record<string, string>;
  setCustomDrafts: React.Dispatch<React.SetStateAction<Record<string, string>>>;
};

function ClarificationCarouselBody({
  sortedMessages,
  group,
  activeIndex,
  setActiveIndex,
  customDrafts,
  setCustomDrafts,
}: CarouselBodyProps) {
  const total = sortedMessages.length;
  const activeMessage = sortedMessages[Math.min(activeIndex, total - 1)] ?? null;
  const meta = activeMessage ? readSingleQuestionMeta(activeMessage) : null;
  const showAgentDisconnectedAtTop = sortedMessages.some(
    (m) => (m.metadata as ClarificationRequestMetadata | undefined)?.agent_disconnected === true,
  );
  const isSubmitting = group.submitState === "submitting";

  const isSingleQuestion = total === 1;

  const allAnswered = sortedMessages.every((m) => {
    const id = readSingleQuestionMeta(m)?.questionId;
    return id ? Boolean(group.answers[id]) : false;
  });

  const handleSubmit = useCallback(() => {
    if (allAnswered) void group.submitCollected();
  }, [allAnswered, group]);

  if (!meta) return null;

  const stored = group.answers[meta.questionId];
  const selectedOption =
    stored?.selected_options && stored.selected_options.length > 0
      ? stored.selected_options[0]
      : null;
  const customCommittedText = stored?.custom_text ?? null;
  const draft = customDrafts[meta.questionId] ?? "";

  // Records the new answer, then either auto-submits (single-question case
  // takes the override path so the freshly recorded answer is included even
  // though setState is async) or auto-advances to the next step.
  const commitAnswer = (answer: ClarificationAnswer) => {
    group.recordAnswer(meta.questionId, answer);
    if (isSingleQuestion) {
      void group.submitCollected({ [meta.questionId]: answer });
      return;
    }
    if (activeIndex < total - 1) {
      setActiveIndex(activeIndex + 1);
    }
  };

  const onSelectOption = (optionId: string) => {
    commitAnswer({
      question_id: meta.questionId,
      selected_options: [optionId],
    });
  };
  const onSubmitCustom = (text: string) => {
    if (!text.trim()) return;
    commitAnswer({
      question_id: meta.questionId,
      selected_options: [],
      custom_text: text.trim(),
    });
  };

  return (
    <>
      <ClarificationCard
        meta={meta}
        index={activeIndex}
        total={total}
        selectedOption={selectedOption}
        customCommittedText={customCommittedText}
        customDraft={draft}
        isSubmitting={isSubmitting}
        showAgentDisconnected={activeIndex === 0 && showAgentDisconnectedAtTop}
        onSelectOption={onSelectOption}
        onCustomDraftChange={(value) =>
          setCustomDrafts((prev) => ({ ...prev, [meta.questionId]: value }))
        }
        onSubmitCustom={onSubmitCustom}
      />
      {!isSingleQuestion && (
        <ClarificationCarouselNav
          activeIndex={activeIndex}
          total={total}
          isSubmitting={isSubmitting}
          onPrev={() => setActiveIndex(Math.max(0, activeIndex - 1))}
          onNext={() => setActiveIndex(Math.min(total - 1, activeIndex + 1))}
          onSubmit={handleSubmit}
          canSubmit={allAnswered}
        />
      )}
      <CarouselKeyboardShortcuts
        meta={meta}
        activeIndex={activeIndex}
        total={total}
        canSubmit={allAnswered}
        onPick={(idx) => onSelectOption(meta.question.options[idx].option_id)}
        onPrev={() => setActiveIndex(Math.max(0, activeIndex - 1))}
        onNext={() => setActiveIndex(Math.min(total - 1, activeIndex + 1))}
        onSkip={() => void group.skipAll("User skipped")}
        onSubmit={handleSubmit}
      />
    </>
  );
}

export function ClarificationInputOverlay({
  messages,
  onResolved,
}: ClarificationInputOverlayProps) {
  const sortedMessages = useMemo(
    () => sortMessagesByQuestionIndex(resolveQuestionMessages(messages)),
    [messages],
  );
  const group = useClarificationGroup(sortedMessages);
  const [customDrafts, setCustomDrafts] = useState<Record<string, string>>({});
  const [rawActiveIndex, setActiveIndex] = useState(0);
  // Clamp the active index to the current bundle size so late-arriving
  // messages or shrunk bundles never put us out of range.
  const total = sortedMessages.length;
  const activeIndex = total === 0 ? 0 : Math.min(rawActiveIndex, total - 1);

  useResolveCallback(group.submitState, onResolved);

  if (sortedMessages.length === 0) return null;
  const isSubmitting = group.submitState === "submitting";

  const isAnsweredAt = (index: number) => {
    const m = sortedMessages[index];
    if (!m) return false;
    const id = readSingleQuestionMeta(m)?.questionId;
    return id ? Boolean(group.answers[id]) : false;
  };

  return (
    <div className="relative" data-testid="clarification-overlay">
      <div className="flex items-center justify-between gap-3 px-4 pt-3 pb-2">
        <div className="flex items-center gap-3">
          {total > 1 && (
            <ClarificationStepper
              total={total}
              activeIndex={activeIndex}
              isAnswered={isAnsweredAt}
              onJump={setActiveIndex}
              isSubmitting={isSubmitting}
            />
          )}
          {total > 1 && (
            <span
              data-testid="clarification-group-progress"
              className="text-xs text-muted-foreground"
            >
              {group.answeredCount} of {group.total} answered
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={() => void group.skipAll("User skipped")}
          disabled={isSubmitting}
          className="text-muted-foreground hover:text-foreground cursor-pointer disabled:opacity-50"
          data-testid="clarification-skip"
          aria-label="Skip all questions"
        >
          <IconX className="h-4 w-4" />
        </button>
      </div>
      <ClarificationCarouselBody
        sortedMessages={sortedMessages}
        group={group}
        activeIndex={activeIndex}
        setActiveIndex={setActiveIndex}
        customDrafts={customDrafts}
        setCustomDrafts={setCustomDrafts}
      />
    </div>
  );
}

export type { ClarificationAnswer };
