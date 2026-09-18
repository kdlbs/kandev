"use client";

/* eslint-disable max-lines -- the overlay keeps active and late-answer lifecycle states together. */

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import type {
  Message,
  ClarificationRequestMetadata,
  ClarificationAnswer,
  ClarificationQuestion,
} from "@/lib/types/http";
import {
  useClarificationGroup,
  type ClarificationOutcome,
} from "@/hooks/domains/session/use-clarification-group";
import type { LateClarificationSnapshot } from "@/hooks/use-late-clarification-message";
import type { MessageAdmissionOutcome } from "@/hooks/use-message-handler";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import {
  CLARIFICATION_CUSTOM_TEXT_MAX_RUNES,
  ClarificationCarouselNav,
  ClarificationCustomInput,
  ClarificationOptions,
  countRunes,
} from "./clarification-overlay-parts";
import { ClarificationOverlayTopBar } from "./clarification-overlay-header";
import { ClarificationStatusBanner } from "./clarification-status-banner";
import { ClarificationMarkdown } from "./clarification-markdown";
import { useTranslation } from "react-i18next";

type ClarificationInputOverlayProps = {
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
  keyboardShortcutsEnabled?: boolean;
  /** True when the session no longer has a live clarification waiter. */
  agentDisconnected?: boolean;
  // Called when the user presses Escape. Unlike Skip, this must not answer or
  // reject the bundle — it only dismisses the UI (e.g. collapses the panel).
  // The question stays pending and the agent stays blocked.
  onDismiss: () => void;
  // Called by the expanded header's collapse control.
  onCollapse?: () => void;
  collapseContentId?: string;
  // Additive: reports every settled submission outcome, distinct from
  // onResolved's narrower "this caller's own answer landed" signal. The
  // task session and Quick Chat hosts leave this unset and keep their
  // existing behavior identical; the Needs-you Inbox is the first host that
  // needs to tell "this caller won" apart from "another caller won" /
  // "no longer active" / "submission failed" to decide whether to remove its
  // row (design-02#Failure-and-recovery).
  onOutcome?: (outcome: ClarificationOutcome) => void;
  mode?: "active" | "late";
  onLateAnswer?: (snapshot: LateClarificationSnapshot) => Promise<MessageAdmissionOutcome>;
  /** Restores a late-message draft after its inline form was removed. */
  initialAnswers?: readonly ClarificationAnswer[];
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

function useResetOverlayStateOnBundleChange(
  pendingId: string | null,
  setCustomDrafts: (drafts: Record<string, string>) => void,
  setActiveIndex: (index: number) => void,
) {
  useEffect(() => {
    // The hook resets its answer and retry state when a new pending bundle
    // replaces the current one. Drafts and carousel navigation are overlay-
    // local state, so they need the same lifecycle fence as well. Question IDs
    // can repeat across bundles, which makes an ID-only draft key unsafe.
    setCustomDrafts({});
    setActiveIndex(0);
  }, [pendingId, setCustomDrafts, setActiveIndex]);
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
  agentDisconnected: boolean;
  shortcutScopeRef: RefObject<HTMLElement | null>;
  armedEventRef: RefObject<KeyboardEvent | null>;
  keyboardShortcutsEnabled: boolean;
  onSubmit: () => void;
  submitAnswers: (override?: Record<string, ClarificationAnswer>) => void | Promise<void>;
  autoSubmitSingleQuestion: boolean;
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
  submitAnswers: (override?: Record<string, ClarificationAnswer>) => void | Promise<void>;
  autoSubmitSingleQuestion: boolean;
};

type AnswerSubmitter = (override?: Record<string, ClarificationAnswer>) => void | Promise<void>;

function chooseSubmitter(
  lateMode: boolean,
  lateInteraction: boolean,
  lateSubmitter: AnswerSubmitter,
  lateRetry: AnswerSubmitter,
  activeSubmitter: AnswerSubmitter,
): AnswerSubmitter {
  if (lateMode) return lateSubmitter;
  if (lateInteraction) return lateRetry;
  return activeSubmitter;
}

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
  const {
    meta,
    group,
    isSingleQuestion,
    activeIndex,
    total,
    setActiveIndex,
    setCustomDrafts,
    submitAnswers,
    autoSubmitSingleQuestion,
  } = ctx;

  // Records the answer, then auto-submits (single-question — uses the override
  // path because setState is async) or auto-advances to the next step.
  const commitAnswer = (answer: ClarificationAnswer) => {
    group.recordAnswer(meta.questionId, answer);
    if (isSingleQuestion && autoSubmitSingleQuestion) {
      void submitAnswers({ [meta.questionId]: answer });
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
  agentDisconnected,
  shortcutScopeRef,
  armedEventRef,
  keyboardShortcutsEnabled,
  onSubmit,
  submitAnswers,
  autoSubmitSingleQuestion,
  onDismiss,
}: CarouselBodyProps) {
  const total = sortedMessages.length;
  const showAgentDisconnectedAtTop =
    agentDisconnected ||
    sortedMessages.some(
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
    submitAnswers,
    autoSubmitSingleQuestion,
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

// eslint-disable-next-line max-lines-per-function, complexity, sonarjs/cognitive-complexity -- coordinates the complete clarification overlay lifecycle.
export function ClarificationInputOverlay({
  messages,
  onResolved,
  onOutcome,
  mode = "active",
  onLateAnswer,
  initialAnswers,
  shortcutScopeRef,
  keyboardShortcutsEnabled = true,
  agentDisconnected = false,
  onDismiss,
  onCollapse,
  collapseContentId,
}: ClarificationInputOverlayProps) {
  const { t } = useTranslation();
  const lateMode = mode === "late";
  const sortedMessages = useMemo(
    () => sortMessagesByQuestionIndex(resolveQuestionMessages(messages)),
    [messages],
  );
  const group = useClarificationGroup(sortedMessages, onOutcome, onLateAnswer);
  const [lateStatus, setLateStatus] = useState<"idle" | "sending" | "sent" | "queued" | "error">(
    "idle",
  );
  const [lateSnapshot, setLateSnapshot] = useState<LateClarificationSnapshot | null>(null);
  const isSubmitting =
    group.submitState === "submitting" ||
    group.lateAnswerState === "sending" ||
    lateStatus === "sending";
  const lateInteraction = lateMode || group.lateAnswerState !== "idle";
  const [customDrafts, setCustomDrafts] = useState<Record<string, string>>({});
  const [rawActiveIndex, setActiveIndex] = useState(0);
  useResetOverlayStateOnBundleChange(group.pendingId, setCustomDrafts, setActiveIndex);
  const initialAnswersKey = JSON.stringify(initialAnswers ?? []);
  useEffect(() => {
    if (!initialAnswers || initialAnswers.length === 0) return;
    for (const answer of initialAnswers) {
      group.recordAnswer(answer.question_id, answer);
    }
    setCustomDrafts((current) => {
      const next = { ...current };
      for (const answer of initialAnswers) {
        if (answer.custom_text !== undefined) next[answer.question_id] = answer.custom_text;
      }
      return next;
    });
  }, [group.recordAnswer, initialAnswers, initialAnswersKey]);
  // Clamp the active index to the current bundle size so late-arriving
  // messages or shrunk bundles never put us out of range.
  const total = sortedMessages.length;
  const activeIndex = total === 0 ? 0 : Math.min(rawActiveIndex, total - 1);
  const activeMessage = sortedMessages[activeIndex] ?? null;
  const meta = activeMessage ? readSingleQuestionMeta(activeMessage) : null;
  const sharedContext = readSharedContext(sortedMessages[0]);

  useResolveCallback(lateMode ? "idle" : group.submitState, onResolved);

  // group is a fresh object every render, but its submitCollected callback is
  // memoised by the hook — depend on the function only so this useCallback
  // doesn't churn on every keystroke (via the live-record path).
  const allAnswered = computeAllAnswered(sortedMessages, group.answers);
  const submitLateAnswer = useCallback(
    async (override?: Record<string, ClarificationAnswer>) => {
      const answers = { ...group.answers, ...(override ?? {}) };
      if (!computeAllAnswered(sortedMessages, answers) || !onLateAnswer) return;
      const snapshot: LateClarificationSnapshot = {
        messages: sortedMessages.slice(),
        answers: Object.values(answers),
      };
      setLateSnapshot(snapshot);
      setLateStatus("sending");
      try {
        const outcome = await onLateAnswer(snapshot);
        setLateStatus(outcome);
      } catch {
        setLateStatus("error");
      }
    },
    [group.answers, onLateAnswer, sortedMessages],
  );
  const submitAnswers = chooseSubmitter(
    lateMode,
    lateInteraction,
    submitLateAnswer,
    group.retryLateAnswer,
    group.submitCollected,
  );
  const handleSubmit = useCallback(() => {
    if (allAnswered) void submitAnswers();
  }, [allAnswered, submitAnswers]);
  const retryLateAnswerForm = useCallback(() => {
    if (lateSnapshot) void submitLateAnswer();
  }, [lateSnapshot, submitLateAnswer]);
  const retryLateAnswer = lateMode ? retryLateAnswerForm : group.retryLateAnswer;

  // Gated on the same resolved `meta` that decides whether
  // ClarificationCarouselBody (and CarouselKeyboardShortcuts within it) mount
  // at all, not a looser proxy like sortedMessages.length > 0 -- otherwise the
  // guard could tell the dialog "handledHere" for a state where the widget
  // that would actually handle Escape never mounted in the first place.
  const armedEventRef = useEscapeGuardRegistration(
    keyboardShortcutsEnabled &&
      !isSubmitting &&
      group.submitState !== "expired" &&
      group.lateAnswerState !== "sent" &&
      group.lateAnswerState !== "queued" &&
      (!lateInteraction || (lateStatus !== "sent" && lateStatus !== "queued")) &&
      meta !== null,
    shortcutScopeRef,
  );

  if (sortedMessages.length === 0) return null;

  if (!lateMode && group.submitState === "expired") {
    return (
      <div className="relative" data-testid="clarification-overlay">
        <ClarificationStatusBanner state="expired" onRetry={() => void group.retry()} />
      </div>
    );
  }

  if (
    (lateMode && (lateStatus === "sent" || lateStatus === "queued")) ||
    (!lateMode && (group.lateAnswerState === "sent" || group.lateAnswerState === "queued"))
  ) {
    return (
      <div className="relative" data-testid="clarification-overlay">
        <div
          data-testid="clarification-late-success"
          className="flex min-h-11 items-center justify-between gap-3 px-4 py-2 text-sm text-muted-foreground"
        >
          <span>
            {(lateMode ? lateStatus : group.lateAnswerState) === "sent"
              ? t("task:lateAnswerSent")
              : t("task:lateAnswerQueued")}
          </span>
          <button
            type="button"
            className="min-h-11 cursor-pointer underline underline-offset-2 md:min-h-0"
            onClick={onDismiss}
            data-testid="clarification-late-success-close"
          >
            {t("task:close")}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="relative" data-testid="clarification-overlay">
      <ClarificationOverlayTopBar
        total={total}
        activeIndex={activeIndex}
        isAnswered={(index) => isQuestionAnsweredAt(sortedMessages, group.answers, index)}
        onJump={setActiveIndex}
        isSubmitting={isSubmitting}
        answeredCount={group.answeredCount}
        answerableTotal={group.total}
        allAnswered={allAnswered}
        onSubmit={handleSubmit}
        onSkip={() => void group.skipAll("User skipped")}
        lateMode={lateInteraction}
        onLateClose={onDismiss}
        onCollapse={onCollapse}
        collapseContentId={collapseContentId}
      />
      {group.submitState === "error" && (
        <ClarificationStatusBanner state={group.submitState} onRetry={() => void group.retry()} />
      )}
      {lateInteraction &&
        ((lateMode && lateStatus === "error") ||
          (!lateMode && group.lateAnswerState === "error")) && (
          <ClarificationStatusBanner state="error" onRetry={retryLateAnswer} />
        )}
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
        agentDisconnected={agentDisconnected}
        shortcutScopeRef={shortcutScopeRef}
        armedEventRef={armedEventRef}
        keyboardShortcutsEnabled={keyboardShortcutsEnabled}
        onSubmit={handleSubmit}
        submitAnswers={submitAnswers}
        autoSubmitSingleQuestion={!lateInteraction}
        onDismiss={onDismiss}
      />
    </div>
  );
}

export type { ClarificationAnswer };
