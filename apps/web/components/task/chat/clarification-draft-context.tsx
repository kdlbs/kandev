import {
  createContext,
  useContext,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from "react";
import type { ClarificationAnswer } from "@/lib/types/http";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import {
  answerDraftGeneration,
  canRetainAnswerDrafts,
  readAnswerDraft,
  retireAnswerDraft,
  writeAnswerDraft,
  type AnswerDraftAuth,
  type RetainedAnswerDraft,
} from "@/lib/clarification/answer-draft-cache";

export type ClarificationDraftQuestion = {
  id: string;
  optionIds: readonly string[];
};

type ClarificationPanelSize = { desktop: number | null; phone: number | null };
const EMPTY_PANEL_SIZE: ClarificationPanelSize = { desktop: null, phone: null };
const MAX_CACHED_CLARIFICATION_SIZES = 8;
const EMPTY_AUTH: AnswerDraftAuth = { mode: "disabled", authenticated: true, user: null };
const ResizeContext = createContext<Map<string, ClarificationPanelSize> | null>(null);
const DraftContext = createContext<DraftContextValue | null>(null);

type DraftContextValue = {
  /** Application-store owner, or a page-local owner when no store is mounted. */
  owner: object;
  taskId: string | null;
  generation: number;
  retain: boolean;
};

// The task page supplies task identity and local panel sizes. Answer retention
// belongs to the application store (see the answer-draft ownership decision), so
// navigation and provider unmounts keep unfinished answers.
export function ClarificationDraftProvider({
  children,
  taskId,
}: {
  children: ReactNode;
  taskId: string | null;
}) {
  const store = useOptionalAppStoreApi();
  const [pageOwner] = useState<object>(() => ({}));
  const auth = useOptionalAppStore((state) => state.auth, EMPTY_AUTH);
  const [sizes, setSizes] = useState(() => ({
    taskId,
    cache: new Map<string, ClarificationPanelSize>(),
  }));
  if (sizes.taskId !== taskId) {
    setSizes({ taskId, cache: new Map<string, ClarificationPanelSize>() });
  }
  const owner = store ?? pageOwner;
  const generation = store ? answerDraftGeneration(store) : 0;
  const retain = canRetainAnswerDrafts(auth);
  const draftContext = useMemo(
    () => ({ owner, taskId, generation, retain }),
    [owner, taskId, generation, retain],
  );
  return (
    <DraftContext.Provider value={draftContext}>
      <ResizeContext.Provider value={sizes.cache}>{children}</ResizeContext.Provider>
    </DraftContext.Provider>
  );
}

export function useClarificationResizeState(bundleKey: string | null) {
  const cache = useContext(ResizeContext);
  const [sizes, setSizes] = useState<ClarificationPanelSize>(
    () => (bundleKey ? cache?.get(bundleKey) : undefined) ?? EMPTY_PANEL_SIZE,
  );
  const lastBundleKey = useRef(bundleKey);

  useLayoutEffect(() => {
    if (lastBundleKey.current !== bundleKey) {
      lastBundleKey.current = bundleKey;
      setSizes((bundleKey ? cache?.get(bundleKey) : undefined) ?? EMPTY_PANEL_SIZE);
      return;
    }
    if (!cache || !bundleKey) return;
    cache.delete(bundleKey);
    if (sizes.desktop === null && sizes.phone === null) return;
    cache.set(bundleKey, sizes);
    while (cache.size > MAX_CACHED_CLARIFICATION_SIZES) {
      cache.delete(cache.keys().next().value!);
    }
  }, [bundleKey, cache, sizes]);

  const setDesktopHeight: Dispatch<SetStateAction<number | null>> = useCallback((value) => {
    setSizes((current) => ({
      ...current,
      desktop: typeof value === "function" ? value(current.desktop) : value,
    }));
  }, []);
  const setPhoneHeight: Dispatch<SetStateAction<number | null>> = useCallback((value) => {
    setSizes((current) => ({
      ...current,
      phone: typeof value === "function" ? value(current.phone) : value,
    }));
  }, []);

  return {
    desktopHeight: sizes.desktop,
    phoneHeight: sizes.phone,
    setDesktopHeight,
    setPhoneHeight,
  };
}

function clampIndex(index: number, questionCount: number): number {
  if (questionCount <= 0) return 0;
  return Math.max(0, Math.min(index, questionCount - 1));
}

function restoreAnswers(
  answers: readonly ClarificationAnswer[],
  questions: readonly ClarificationDraftQuestion[],
): readonly ClarificationAnswer[] {
  const optionIdsByQuestion = new Map(
    questions.map((question) => [question.id, question.optionIds]),
  );
  return answers.filter((answer) => {
    const optionIds = optionIdsByQuestion.get(answer.question_id);
    if (!optionIds) return false;
    return (answer.selected_options ?? []).every((optionId) => optionIds.includes(optionId));
  });
}

function restoreCustomDrafts(
  customDrafts: Record<string, string>,
  questions: readonly ClarificationDraftQuestion[],
): Record<string, string> {
  const questionIds = new Set(questions.map((question) => question.id));
  const restored: Record<string, string> = {};
  for (const [questionId, text] of Object.entries(customDrafts)) {
    if (questionIds.has(questionId)) restored[questionId] = text;
  }
  return restored;
}

function restoreDraft(
  draft: RetainedAnswerDraft,
  questions: readonly ClarificationDraftQuestion[] | undefined,
): RetainedAnswerDraft {
  if (!questions) return draft;
  return {
    customDrafts: restoreCustomDrafts(draft.customDrafts, questions),
    answers: restoreAnswers(draft.answers, questions),
    activeIndex: clampIndex(draft.activeIndex, questions.length),
  };
}

type DraftState = {
  scopeKey: string | null;
  generation: number;
  /** True once the form has taken over the restored answers. */
  consumedAnswers: boolean;
  customDrafts: Record<string, string>;
  activeIndex: number;
  pendingAnswers: readonly ClarificationAnswer[];
  /** Answers captured from the previous form identity, ignored for persistence. */
  staleAnswers: Record<string, ClarificationAnswer> | null;
};

function draftStateFor(
  scopeKey: string | null,
  generation: number,
  restored: RetainedAnswerDraft | undefined,
  answers: Record<string, ClarificationAnswer>,
): DraftState {
  return {
    scopeKey,
    generation,
    consumedAnswers: false,
    customDrafts: restored?.customDrafts ?? {},
    activeIndex: restored?.activeIndex ?? 0,
    pendingAnswers: restored?.answers ?? [],
    staleAnswers: answers,
  };
}

function nextDraftState(
  state: DraftState,
  scopeKey: string | null,
  generation: number,
  restored: RetainedAnswerDraft | undefined,
  answers: Record<string, ClarificationAnswer>,
): DraftState | null {
  // A scope or authority-generation change quarantines the current answer map
  // and starts the form empty. The overlay clears the group's answers so a
  // fresh interaction can still be retained under the new generation.
  if (state.scopeKey !== scopeKey || state.generation !== generation) {
    return draftStateFor(scopeKey, generation, restored, answers);
  }
  return null;
}

function visibleDraft(
  state: DraftState,
  questions: readonly ClarificationDraftQuestion[] | undefined,
): { customDrafts: Record<string, string>; activeIndex: number } {
  if (!questions) return { customDrafts: state.customDrafts, activeIndex: state.activeIndex };
  return {
    customDrafts: restoreCustomDrafts(state.customDrafts, questions),
    activeIndex: clampIndex(state.activeIndex, questions.length),
  };
}

function liveAnswersConsumed(
  state: DraftState,
  answers: Record<string, ClarificationAnswer>,
): boolean {
  if (state.consumedAnswers) return true;
  return state.staleAnswers !== answers && Object.keys(answers).length > 0;
}

function persistedAnswers(
  state: DraftState,
  answers: Record<string, ClarificationAnswer>,
  consumedAnswers: boolean,
): readonly ClarificationAnswer[] {
  if (state.staleAnswers === answers) return state.pendingAnswers;
  const liveAnswers = Object.values(answers);
  if (liveAnswers.length > 0) return liveAnswers;
  return consumedAnswers ? [] : state.pendingAnswers;
}

function hasMeaningfulCustomText(customDrafts: Record<string, string>): boolean {
  return Object.values(customDrafts).some((text) => text.trim().length > 0);
}

function draftWriteContext(context: DraftContextValue | null, bundleKey: string | null) {
  const identity =
    context?.taskId && bundleKey && context.retain
      ? JSON.stringify([context.taskId, bundleKey])
      : null;
  return {
    identity,
    // Without a task provider the form keeps today's component-local lifetime:
    // a bundle change still resets the visible draft.
    scopeKey: identity ?? (bundleKey ? `local:${bundleKey}` : null),
    owner: context?.owner ?? null,
    generation: context?.generation ?? 0,
  };
}

export function useClarificationDraft(
  bundleKey: string | null,
  answers: Record<string, ClarificationAnswer>,
  settled: boolean,
  questions?: readonly ClarificationDraftQuestion[],
) {
  const context = useContext(DraftContext);
  const { identity, scopeKey, owner, generation } = draftWriteContext(context, bundleKey);
  const saved = useMemo(
    () => (identity && owner ? readAnswerDraft(owner, identity) : undefined),
    [identity, owner, generation],
  );
  const restored = useMemo(
    () => (saved ? restoreDraft(saved, questions) : undefined),
    [saved, questions],
  );
  const [state, setState] = useState(() => draftStateFor(scopeKey, generation, restored, answers));
  const adjusted = nextDraftState(state, scopeKey, generation, restored, answers);
  if (adjusted) setState(adjusted);

  const setCustomDrafts: Dispatch<SetStateAction<Record<string, string>>> = useCallback((value) => {
    setState((current) => ({
      ...current,
      customDrafts: typeof value === "function" ? value(current.customDrafts) : value,
    }));
  }, []);
  const setActiveIndex: Dispatch<SetStateAction<number>> = useCallback((value) => {
    setState((current) => ({
      ...current,
      activeIndex: typeof value === "function" ? value(current.activeIndex) : value,
    }));
  }, []);

  const consumedAnswers = liveAnswersConsumed(state, answers);
  const answersToPersist = persistedAnswers(state, answers, consumedAnswers);
  const { customDrafts, activeIndex } = visibleDraft(state, questions);
  const hasContent =
    hasMeaningfulCustomText(customDrafts) || answersToPersist.length > 0 || activeIndex > 0;
  useEffect(() => {
    if (!consumedAnswers || state.consumedAnswers) return;
    setState((current) => ({ ...current, consumedAnswers }));
  }, [consumedAnswers, state.consumedAnswers]);
  useLayoutEffect(() => {
    if (!identity || !owner) return;
    if (settled || !hasContent) {
      retireAnswerDraft(owner, identity, generation);
      return;
    }
    writeAnswerDraft(owner, identity, generation, {
      customDrafts,
      activeIndex,
      answers: answersToPersist,
    });
  }, [
    activeIndex,
    answersToPersist,
    customDrafts,
    generation,
    hasContent,
    identity,
    owner,
    settled,
  ]);

  return {
    customDrafts,
    setCustomDrafts,
    rawActiveIndex: activeIndex,
    setActiveIndex,
    savedAnswers: state.pendingAnswers.length > 0 ? state.pendingAnswers : undefined,
    generation,
  };
}

/**
 * Drops the previous authority's local answers when the answer cache starts a
 * new generation. The hook's own text/position reset happens during render; the
 * group answers live above it, so the overlay clears them here before admitting
 * fresh input under the current generation.
 */
export function useClarificationAuthorityReset(
  generation: number,
  answers: Record<string, ClarificationAnswer>,
  clearAnswer: (questionId: string) => void,
) {
  const appliedGeneration = useRef(generation);
  useEffect(() => {
    if (appliedGeneration.current === generation) return;
    appliedGeneration.current = generation;
    for (const questionId of Object.keys(answers)) clearAnswer(questionId);
  }, [answers, clearAnswer, generation]);
}
