"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconX, IconMessageQuestion, IconInfoCircle, IconCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
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
  keyboardShortcutsEnabled?: boolean;
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

function isQuestionAnsweredAt(
  messages: readonly Message[],
  answers: Record<string, ClarificationAnswer>,
  index: number,
): boolean {
  const message = messages[index];
  if (!message) return false;
  const questionId = readSingleQuestionMeta(message)?.questionId;
  return questionId ? Boolean(answers[questionId]) : false;
}

type CardProps = {
  meta: SingleQuestionMeta;
  index: number;
  total: number;
  selectedOption: string | null;
  customCommittedText: string | null;
  customDraft: string;
  customActive: boolean;
  isSubmitting: boolean;
  showAgentDisconnected: boolean;
  onSelectOption: (optionId: string) => void;
  onCustomDraftChange: (text: string) => void;
  onSubmitCustom: (text: string) => void;
  onRequestFinalSubmit: () => void;
};

function ClarificationCard(props: CardProps) {
  const {
    meta,
    index,
    total,
    selectedOption,
    customCommittedText,
    customDraft,
    customActive,
    isSubmitting,
    showAgentDisconnected,
    onSelectOption,
    onCustomDraftChange,
    onSubmitCustom,
    onRequestFinalSubmit,
  } = props;
  const { question, metadata } = meta;
  return (
    <div
      data-testid="clarification-question-card"
      data-question-id={meta.questionId}
      data-question-index={String(index)}
      className="px-4 pt-1 pb-4"
    >
      {(total > 1 || metadata.question.title) && (
        <div className="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
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
      )}
      <div className="markdown-body max-w-none text-sm font-medium [&>*:first-child]:mt-0 [&>*:last-child]:mb-0 mb-3">
        <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
          {question.prompt}
        </ReactMarkdown>
      </div>
      <ClarificationOptions
        options={question.options}
        selectedOption={selectedOption}
        isSubmitting={isSubmitting}
        customActive={customActive}
        onSelectOption={onSelectOption}
      />
      {showAgentDisconnected && (
        <div
          data-testid="clarification-deferred-notice"
          className="mt-2 flex items-center gap-1.5 text-xs text-slate-600 dark:text-slate-400"
        >
          <IconInfoCircle className="h-3.5 w-3.5 flex-shrink-0" />
          The agent has moved on. Your response will be sent as a new message.
        </div>
      )}
      <ClarificationCustomInput
        draft={customDraft}
        isSubmitting={isSubmitting}
        committedText={customCommittedText}
        active={customActive}
        onChange={onCustomDraftChange}
        onSubmit={onSubmitCustom}
        onRequestFinalSubmit={onRequestFinalSubmit}
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
  enabled: boolean;
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

// tryHandleMetaEnter returns true when the event was Cmd/Ctrl+Enter, so the
// caller can short-circuit. When focus is inside the custom-text input it
// returns true *without* invoking onSubmit — the input's own keydown handler
// owns that path and is responsible for committing the draft + final submit.
function tryHandleMetaEnter(e: KeyboardEvent, canSubmit: boolean, onSubmit: () => void): boolean {
  if (e.key !== "Enter" || e.shiftKey || e.altKey) return false;
  if (!e.metaKey && !e.ctrlKey) return false;
  const inEditable =
    e.target instanceof HTMLElement &&
    (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA");
  if (inEditable) return true;
  e.preventDefault();
  if (canSubmit) onSubmit();
  return true;
}

function CarouselKeyboardShortcuts(args: CarouselShortcutArgs) {
  const { enabled } = args;
  const optionsCount = args.meta.question.options.length;
  const isLast = args.activeIndex === args.total - 1;
  const { canSubmit, onPick, onPrev, onNext, onSkip, onSubmit } = args;
  useEffect(() => {
    if (!enabled) return;
    const onKey = (e: KeyboardEvent) => {
      if (tryHandleMetaEnter(e, canSubmit, onSubmit)) return;
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
  }, [enabled, optionsCount, isLast, canSubmit, onPick, onPrev, onNext, onSkip, onSubmit]);
  return null;
}

type CarouselBodyProps = {
  sortedMessages: Message[];
  group: ReturnType<typeof useClarificationGroup>;
  activeIndex: number;
  setActiveIndex: (idx: number) => void;
  customDrafts: Record<string, string>;
  setCustomDrafts: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  allAnswered: boolean;
  isSubmitting: boolean;
  keyboardShortcutsEnabled: boolean;
  onSubmit: () => void;
};

type QuestionHandlerCtx = {
  meta: SingleQuestionMeta;
  group: ReturnType<typeof useClarificationGroup>;
  isSingleQuestion: boolean;
  activeIndex: number;
  total: number;
  setActiveIndex: (idx: number) => void;
  setCustomDrafts: React.Dispatch<React.SetStateAction<Record<string, string>>>;
};

type QuestionHandlers = {
  onSelectOption: (optionId: string) => void;
  onCustomDraftChange: (value: string) => void;
  onSubmitCustom: (text: string) => void;
};

type SelectionState = {
  selectedOption: string | null;
  customCommittedText: string | null;
  draft: string;
  customActive: boolean;
};

function deriveSelectionState(
  meta: SingleQuestionMeta,
  answers: Record<string, ClarificationAnswer>,
  customDrafts: Record<string, string>,
): SelectionState {
  const stored = answers[meta.questionId];
  const selected = stored?.selected_options ?? [];
  const selectedOption = selected.length > 0 ? selected[0] : null;
  const customCommittedText = stored?.custom_text ?? null;
  const draft = customDrafts[meta.questionId] ?? "";
  const hasCustomText = draft.trim().length > 0 || (customCommittedText?.length ?? 0) > 0;
  const customActive = !selectedOption && hasCustomText;
  return { selectedOption, customCommittedText, draft, customActive };
}

function buildQuestionHandlers(ctx: QuestionHandlerCtx): QuestionHandlers {
  const { meta, group, isSingleQuestion, activeIndex, total, setActiveIndex, setCustomDrafts } =
    ctx;

  // Records the answer, then auto-submits (single-question — uses the override
  // path because setState is async) or auto-advances to the next step.
  const commitAnswer = (answer: ClarificationAnswer) => {
    group.recordAnswer(meta.questionId, answer);
    if (isSingleQuestion) {
      void group.submitCollected({ [meta.questionId]: answer });
      return;
    }
    if (activeIndex < total - 1) setActiveIndex(activeIndex + 1);
  };

  return {
    onSelectOption(optionId) {
      // Picking an option wipes any in-flight draft so the answer state and
      // the visible input agree (custom_text and selected_options are mutually
      // exclusive at commit time).
      setCustomDrafts((prev) => {
        if (!prev[meta.questionId]) return prev;
        return { ...prev, [meta.questionId]: "" };
      });
      commitAnswer({ question_id: meta.questionId, selected_options: [optionId] });
    },
    // Live-record the draft so the stepper updates and the custom input lights
    // up the moment the user types. Emptying the draft clears the answer so
    // allAnswered reverts to false. Enter/Cmd+Enter still drives advance/submit.
    onCustomDraftChange(value) {
      setCustomDrafts((prev) => ({ ...prev, [meta.questionId]: value }));
      const trimmed = value.trim();
      if (trimmed.length === 0) {
        group.clearAnswer(meta.questionId);
        return;
      }
      group.recordAnswer(meta.questionId, {
        question_id: meta.questionId,
        selected_options: [],
        custom_text: trimmed,
      });
    },
    onSubmitCustom(text) {
      const trimmed = text.trim();
      if (!trimmed) return;
      commitAnswer({
        question_id: meta.questionId,
        selected_options: [],
        custom_text: trimmed,
      });
    },
  };
}

function ClarificationCarouselBody({
  sortedMessages,
  group,
  activeIndex,
  setActiveIndex,
  customDrafts,
  setCustomDrafts,
  allAnswered,
  isSubmitting,
  keyboardShortcutsEnabled,
  onSubmit,
}: CarouselBodyProps) {
  const total = sortedMessages.length;
  const activeMessage = sortedMessages[Math.min(activeIndex, total - 1)] ?? null;
  const meta = activeMessage ? readSingleQuestionMeta(activeMessage) : null;
  const showAgentDisconnectedAtTop = sortedMessages.some(
    (m) => (m.metadata as ClarificationRequestMetadata | undefined)?.agent_disconnected === true,
  );
  const isSingleQuestion = total === 1;

  if (!meta) return null;

  const { selectedOption, customCommittedText, draft, customActive } = deriveSelectionState(
    meta,
    group.answers,
    customDrafts,
  );

  const { onSelectOption, onCustomDraftChange, onSubmitCustom } = buildQuestionHandlers({
    meta,
    group,
    isSingleQuestion,
    activeIndex,
    total,
    setActiveIndex,
    setCustomDrafts,
  });

  return (
    <>
      <ClarificationCard
        meta={meta}
        index={activeIndex}
        total={total}
        selectedOption={selectedOption}
        customCommittedText={customCommittedText}
        customDraft={draft}
        customActive={customActive}
        isSubmitting={isSubmitting}
        showAgentDisconnected={activeIndex === 0 && showAgentDisconnectedAtTop}
        onSelectOption={onSelectOption}
        onCustomDraftChange={onCustomDraftChange}
        onSubmitCustom={onSubmitCustom}
        onRequestFinalSubmit={onSubmit}
      />
      {!isSingleQuestion && (
        <ClarificationCarouselNav
          activeIndex={activeIndex}
          total={total}
          isSubmitting={isSubmitting}
          onPrev={() => setActiveIndex(Math.max(0, activeIndex - 1))}
          onNext={() => setActiveIndex(Math.min(total - 1, activeIndex + 1))}
        />
      )}
      <CarouselKeyboardShortcuts
        enabled={keyboardShortcutsEnabled}
        meta={meta}
        activeIndex={activeIndex}
        total={total}
        canSubmit={allAnswered}
        onPick={(idx) => onSelectOption(meta.question.options[idx].option_id)}
        onPrev={() => setActiveIndex(Math.max(0, activeIndex - 1))}
        onNext={() => setActiveIndex(Math.min(total - 1, activeIndex + 1))}
        onSkip={() => void group.skipAll("User skipped")}
        onSubmit={onSubmit}
      />
    </>
  );
}

export function ClarificationInputOverlay({
  messages,
  onResolved,
  keyboardShortcutsEnabled = true,
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

  // group is a fresh object every render, but its submitCollected callback is
  // memoised by the hook — depend on the function only so this useCallback
  // doesn't churn on every keystroke (via the live-record path).
  const submitCollected = group.submitCollected;
  const allAnswered =
    sortedMessages.length > 0 &&
    sortedMessages.every((m) => {
      const id = readSingleQuestionMeta(m)?.questionId;
      return id ? Boolean(group.answers[id]) : false;
    });
  const handleSubmit = useCallback(() => {
    if (allAnswered) void submitCollected();
  }, [allAnswered, submitCollected]);

  if (sortedMessages.length === 0) return null;
  const isSubmitting = group.submitState === "submitting";

  return (
    <div className="relative" data-testid="clarification-overlay">
      <div className="flex items-center justify-between gap-3 px-4 pt-2 pb-1">
        <div className="flex items-center gap-3 min-w-0">
          <IconMessageQuestion className="h-4 w-4 text-blue-500 flex-shrink-0" />
          {total > 1 && (
            <ClarificationStepper
              total={total}
              activeIndex={activeIndex}
              isAnswered={(index) => isQuestionAnsweredAt(sortedMessages, group.answers, index)}
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
        <div className="flex items-center gap-2">
          {total > 1 && (
            <button
              type="button"
              onClick={handleSubmit}
              disabled={!allAnswered || isSubmitting}
              data-testid="clarification-submit"
              className={cn(
                "inline-flex items-center gap-1 text-xs px-3 py-1 rounded font-medium transition-colors",
                allAnswered && !isSubmitting
                  ? "bg-blue-500 text-white hover:bg-blue-500/90 cursor-pointer"
                  : "bg-muted text-muted-foreground cursor-not-allowed",
              )}
            >
              {isSubmitting ? "Submitting…" : "Submit"}
              <IconCheck className="h-3 w-3" />
            </button>
          )}
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
      </div>
      <ClarificationCarouselBody
        sortedMessages={sortedMessages}
        group={group}
        activeIndex={activeIndex}
        setActiveIndex={setActiveIndex}
        customDrafts={customDrafts}
        setCustomDrafts={setCustomDrafts}
        allAnswered={allAnswered}
        isSubmitting={isSubmitting}
        keyboardShortcutsEnabled={keyboardShortcutsEnabled}
        onSubmit={handleSubmit}
      />
    </div>
  );
}

export type { ClarificationAnswer };
