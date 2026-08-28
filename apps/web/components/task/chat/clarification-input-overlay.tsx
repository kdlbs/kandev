"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { IconMessageQuestion, IconInfoCircle } from "@tabler/icons-react";
import type {
  Message,
  ClarificationRequestMetadata,
  ClarificationAnswer,
  ClarificationQuestion,
} from "@/lib/types/http";
import { useClarificationGroup } from "@/hooks/domains/session/use-clarification-group";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import {
  CLARIFICATION_CUSTOM_TEXT_MAX_RUNES,
  ClarificationCarouselNav,
  ClarificationCustomInput,
  ClarificationOptions,
  ClarificationStepper,
  countRunes,
} from "./clarification-overlay-parts";
import { ClarificationHeaderActions } from "./clarification-overlay-header";
import { ClarificationMarkdown } from "./clarification-markdown";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type ClarificationInputOverlayProps = {
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
  keyboardShortcutsEnabled?: boolean;
  // Called when the user presses Escape. Unlike Skip, this must not answer or
  // reject the bundle — it only dismisses the UI (e.g. collapses the panel).
  // The question stays pending and the agent stays blocked.
  onDismiss: () => void;
  // Called by the expanded header's collapse control.
  onCollapse?: () => void;
  collapseContentId?: string;
};

type SingleQuestionMeta = {
  message: Message;
  metadata: ClarificationRequestMetadata;
  question: ClarificationQuestion;
  questionId: string;
};

function clarificationHeaderClassName(total: number): string {
  return cn(
    "flex min-h-11 justify-between",
    total > 1
      ? "flex-col items-stretch gap-2 px-3 py-2 md:flex-row md:items-center md:gap-3 md:px-4 md:py-0"
      : "items-center gap-3 px-4",
  );
}

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

function readSharedContext(message: Message | undefined): string | null {
  const context = (message?.metadata as ClarificationRequestMetadata | undefined)?.context;
  return context?.trim() ? context : null;
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

function computeAllAnswered(
  sortedMessages: Message[],
  answers: Record<string, ClarificationAnswer>,
): boolean {
  return (
    sortedMessages.length > 0 &&
    sortedMessages.every((m) => {
      const id = readSingleQuestionMeta(m)?.questionId;
      return id ? Boolean(answers[id]) : false;
    })
  );
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
  const { t } = useTranslation();
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
              {t("task:questionOfTotal", { index: index + 1, total })}
            </span>
          )}
          {metadata.question.title && (
            <span data-testid="clarification-question-title" className="text-muted-foreground/70">
              {total > 1 ? "· " : ""}
              <ClarificationMarkdown variant="inline">
                {metadata.question.title}
              </ClarificationMarkdown>
            </span>
          )}
        </div>
      )}
      <ClarificationMarkdown
        variant="block"
        className="mb-3 max-w-none text-sm font-medium [&>*:first-child]:mt-0 [&>*:last-child]:mb-0"
      >
        {question.prompt}
      </ClarificationMarkdown>
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
          {t("task:theAgentHasMovedOn")}
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

// Tells an ancestor dialog (Quick Chat) whether this widget will actually act
// on a given Escape keydown, so the dialog never swallows an Escape that
// nothing here is going to handle. Mirrors CarouselKeyboardShortcuts's own
// gate exactly (enabled, in-scope target, no modifier, not already claimed)
// instead of a separately-derived approximation that could drift out of sync
// with it.
//
// Also records which exact event object this predicate armed. Radix's
// DismissableLayer intercepts Escape on `document` in the capture phase --
// before CarouselKeyboardShortcuts's own bubble-phase `window` listener runs
// -- and the dialog calls event.preventDefault() itself right after this
// predicate returns true. By the time the window listener sees the event,
// e.defaultPrevented is therefore already true for every Escape this
// predicate armed, indistinguishable by flag alone from an unrelated in-scope
// consumer (cancelling a queued-message edit, closing an @-mention popup)
// having already claimed it first. The recorded event reference lets the
// window listener tell those two cases apart: the same object means it was
// this predicate's own doing.
function useEscapeGuardRegistration(
  handledHere: boolean,
  shortcutScopeRef: RefObject<HTMLElement | null>,
) {
  const armedEventRef = useRef<KeyboardEvent | null>(null);
  const testEscapeGuard = useCallback(
    (event: KeyboardEvent) => {
      const armed =
        handledHere &&
        isWithinScope(event.target, shortcutScopeRef) &&
        !shouldIgnoreEscape(event) &&
        !event.defaultPrevented;
      if (armed) armedEventRef.current = event;
      return armed;
    },
    [handledHere, shortcutScopeRef],
  );
  useClarificationEscapeGuard(testEscapeGuard);
  return armedEventRef;
}

type CarouselShortcutArgs = {
  enabled: boolean;
  scopeRef: RefObject<HTMLElement | null>;
  armedEventRef: RefObject<KeyboardEvent | null>;
  meta: SingleQuestionMeta;
  activeIndex: number;
  total: number;
  canSubmit: boolean;
  onPick: (index: number) => void;
  onPrev: () => void;
  onNext: () => void;
  onDismiss: () => void;
  onSubmit: () => void;
};

function isEditableTarget(target: EventTarget | null): target is HTMLElement {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA")
  );
}

// shouldIgnoreShortcut filters out events that the overlay must not handle:
// keystrokes inside an editable control (the user is typing) and any modifier
// combo (so we don't hijack browser shortcuts like Cmd/Ctrl+1..9 for tab
// switching or Alt+ArrowLeft for back-navigation).
function shouldIgnoreShortcut(e: KeyboardEvent): boolean {
  if (isEditableTarget(e.target)) return true;
  return e.metaKey || e.ctrlKey || e.altKey || e.shiftKey;
}

// Escape does not insert a character, so unlike the other shortcuts it must
// still collapse the panel while focus is in the composer or another
// editable control -- the ordinary state right after sending the message
// that triggered the clarification. Only an actual modifier combo blocks it.
function shouldIgnoreEscape(e: KeyboardEvent): boolean {
  return e.metaKey || e.ctrlKey || e.altKey || e.shiftKey;
}

function isWithinScope(
  target: EventTarget | null,
  scopeRef: RefObject<HTMLElement | null>,
): boolean {
  return target instanceof Node && Boolean(scopeRef.current?.contains(target));
}

// tryHandleMetaEnter returns true when the event was Cmd/Ctrl+Enter, so the
// caller can short-circuit. When focus is inside the custom-text input it
// returns true *without* invoking onSubmit — the input's own keydown handler
// owns that path and is responsible for committing the draft + final submit.
function tryHandleMetaEnter(e: KeyboardEvent, canSubmit: boolean, onSubmit: () => void): boolean {
  if (e.key !== "Enter" || e.shiftKey || e.altKey) return false;
  if (!e.metaKey && !e.ctrlKey) return false;
  if (isEditableTarget(e.target)) return true;
  e.preventDefault();
  if (canSubmit) onSubmit();
  return true;
}

function CarouselKeyboardShortcuts(args: CarouselShortcutArgs) {
  const { enabled, scopeRef, armedEventRef } = args;
  const optionsCount = args.meta.question.options.length;
  const isLast = args.activeIndex === args.total - 1;
  const { canSubmit, onPick, onPrev, onNext, onDismiss, onSubmit } = args;
  useEffect(() => {
    if (!enabled) return;
    const onKey = (e: KeyboardEvent) => {
      if (!isWithinScope(e.target, scopeRef)) return;
      if (tryHandleMetaEnter(e, canSubmit, onSubmit)) return;
      if (e.key === "Escape") {
        if (shouldIgnoreEscape(e)) return;
        // Bail if some other in-scope consumer (cancelling a queued-message
        // edit, closing an @-mention/slash-command popup) already claimed
        // this Escape -- but not if the only thing that claimed it was our
        // own paired guard predicate (see useEscapeGuardRegistration), which
        // always runs first and always sets e.defaultPrevented for events it
        // arms.
        if (e.defaultPrevented && armedEventRef.current !== e) return;
        e.preventDefault();
        onDismiss();
        return;
      }
      if (shouldIgnoreShortcut(e)) return;
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
  }, [
    enabled,
    scopeRef,
    armedEventRef,
    optionsCount,
    isLast,
    canSubmit,
    onPick,
    onPrev,
    onNext,
    onDismiss,
    onSubmit,
  ]);
  return null;
}

type CarouselBodyProps = {
  sortedMessages: Message[];
  meta: SingleQuestionMeta | null;
  group: ReturnType<typeof useClarificationGroup>;
  activeIndex: number;
  setActiveIndex: (idx: number) => void;
  customDrafts: Record<string, string>;
  setCustomDrafts: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  allAnswered: boolean;
  isSubmitting: boolean;
  shortcutScopeRef: RefObject<HTMLElement | null>;
  armedEventRef: RefObject<KeyboardEvent | null>;
  keyboardShortcutsEnabled: boolean;
  onSubmit: () => void;
  onDismiss: () => void;
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
    // An over-limit draft is also treated as unanswered (W4): otherwise it
    // could sit recorded from live-typing and reach the header Submit button,
    // which has no per-question rune check of its own, and fail the request
    // with an opaque 400 the user never saw coming from this input.
    onCustomDraftChange(value) {
      setCustomDrafts((prev) => ({ ...prev, [meta.questionId]: value }));
      const trimmed = value.trim();
      if (trimmed.length === 0 || countRunes(trimmed) > CLARIFICATION_CUSTOM_TEXT_MAX_RUNES) {
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
  meta,
  group,
  activeIndex,
  setActiveIndex,
  customDrafts,
  setCustomDrafts,
  allAnswered,
  isSubmitting,
  shortcutScopeRef,
  armedEventRef,
  keyboardShortcutsEnabled,
  onSubmit,
  onDismiss,
}: CarouselBodyProps) {
  const total = sortedMessages.length;
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
        enabled={keyboardShortcutsEnabled && !isSubmitting}
        scopeRef={shortcutScopeRef}
        armedEventRef={armedEventRef}
        meta={meta}
        activeIndex={activeIndex}
        total={total}
        canSubmit={allAnswered}
        onPick={(idx) => onSelectOption(meta.question.options[idx].option_id)}
        onPrev={() => setActiveIndex(Math.max(0, activeIndex - 1))}
        onNext={() => setActiveIndex(Math.min(total - 1, activeIndex + 1))}
        onDismiss={onDismiss}
        onSubmit={onSubmit}
      />
    </>
  );
}

export function ClarificationInputOverlay({
  messages,
  onResolved,
  shortcutScopeRef,
  keyboardShortcutsEnabled = true,
  onDismiss,
  onCollapse,
  collapseContentId,
}: ClarificationInputOverlayProps) {
  const sortedMessages = useMemo(
    () => sortMessagesByQuestionIndex(resolveQuestionMessages(messages)),
    [messages],
  );
  const group = useClarificationGroup(sortedMessages);
  const isSubmitting = group.submitState === "submitting";
  const [customDrafts, setCustomDrafts] = useState<Record<string, string>>({});
  const [rawActiveIndex, setActiveIndex] = useState(0);
  // Clamp the active index to the current bundle size so late-arriving
  // messages or shrunk bundles never put us out of range.
  const total = sortedMessages.length;
  const activeIndex = total === 0 ? 0 : Math.min(rawActiveIndex, total - 1);
  const activeMessage = sortedMessages[activeIndex] ?? null;
  const meta = activeMessage ? readSingleQuestionMeta(activeMessage) : null;
  const sharedContext = readSharedContext(sortedMessages[0]);

  useResolveCallback(group.submitState, onResolved);

  // group is a fresh object every render, but its submitCollected callback is
  // memoised by the hook — depend on the function only so this useCallback
  // doesn't churn on every keystroke (via the live-record path).
  const submitCollected = group.submitCollected;
  const allAnswered = computeAllAnswered(sortedMessages, group.answers);
  const handleSubmit = useCallback(() => {
    if (allAnswered) void submitCollected();
  }, [allAnswered, submitCollected]);

  // Gated on the same resolved `meta` that decides whether
  // ClarificationCarouselBody (and CarouselKeyboardShortcuts within it) mount
  // at all, not a looser proxy like sortedMessages.length > 0 -- otherwise the
  // guard could tell the dialog "handledHere" for a state where the widget
  // that would actually handle Escape never mounted in the first place.
  const armedEventRef = useEscapeGuardRegistration(
    keyboardShortcutsEnabled && !isSubmitting && meta !== null,
    shortcutScopeRef,
  );

  if (sortedMessages.length === 0) return null;

  return (
    <div className="relative" data-testid="clarification-overlay">
      <div
        className={clarificationHeaderClassName(total)}
        data-testid="clarification-overlay-header"
      >
        <div className="flex min-w-0 flex-1 items-center gap-3">
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
              className="ml-auto min-w-0 truncate text-xs text-muted-foreground md:ml-0"
            >
              {group.answeredCount} of {group.total} answered
            </span>
          )}
        </div>
        <ClarificationHeaderActions
          total={total}
          allAnswered={allAnswered}
          isSubmitting={isSubmitting}
          onSubmit={handleSubmit}
          onSkip={() => void group.skipAll("User skipped")}
          onCollapse={onCollapse}
          collapseContentId={collapseContentId}
        />
      </div>
      {sharedContext && (
        <div
          data-testid="clarification-context"
          className="mx-4 mt-3 mb-2 break-words whitespace-pre-wrap text-[13px]"
        >
          {sharedContext}
        </div>
      )}
      <ClarificationCarouselBody
        sortedMessages={sortedMessages}
        meta={meta}
        group={group}
        activeIndex={activeIndex}
        setActiveIndex={setActiveIndex}
        customDrafts={customDrafts}
        setCustomDrafts={setCustomDrafts}
        allAnswered={allAnswered}
        isSubmitting={isSubmitting}
        shortcutScopeRef={shortcutScopeRef}
        armedEventRef={armedEventRef}
        keyboardShortcutsEnabled={keyboardShortcutsEnabled}
        onSubmit={handleSubmit}
        onDismiss={onDismiss}
      />
    </div>
  );
}

export type { ClarificationAnswer };
