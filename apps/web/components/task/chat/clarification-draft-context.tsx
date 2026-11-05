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
  answerDraftIdentity,
  canRetainAnswerDrafts,
  readAnswerDraft,
  retireAnswerDraft,
  writeAnswerDraft,
  type AnswerDraftAuth,
  type RetainedAnswerDraft,
} from "@/lib/clarification/answer-draft-cache";
import {
  emptyClarificationPanelSizes,
  isEmptyClarificationPanelSizes,
  type ClarificationPanelSize,
  type ClarificationPanelSizes,
} from "@/hooks/use-resizable-clarification-overlay";

export type ClarificationDraftQuestion = {
  id: string;
  optionIds: readonly string[];
};

export type ClarificationDraftOptions = {
  questions?: readonly ClarificationDraftQuestion[];
  /** Total questions announced by the bundle metadata. */
  questionTotal?: number;
  /** Only active pending forms use the shared navigation cache. */
  mode?: "active" | "late";
};

type ClarificationPanelSizeCache = Map<string, ClarificationPanelSizes>;
const MAX_CACHED_CLARIFICATION_SIZES = 8;
const EMPTY_AUTH: AnswerDraftAuth = { mode: "disabled", authenticated: true, user: null };
const ResizeContext = createContext<ClarificationPanelSizeCache | null>(null);
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
    cache: new Map<string, ClarificationPanelSizes>(),
  }));
  if (sizes.taskId !== taskId) {
    setSizes({ taskId, cache: new Map<string, ClarificationPanelSizes>() });
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
  const [sizes, setSizes] = useState<ClarificationPanelSizes>(
    () => (bundleKey ? cache?.get(bundleKey) : undefined) ?? emptyClarificationPanelSizes(),
  );
  const lastBundleKey = useRef(bundleKey);

  useLayoutEffect(() => {
    if (lastBundleKey.current !== bundleKey) {
      lastBundleKey.current = bundleKey;
      setSizes((bundleKey ? cache?.get(bundleKey) : undefined) ?? emptyClarificationPanelSizes());
      return;
    }
    if (!cache || !bundleKey) return;
    cache.delete(bundleKey);
    if (isEmptyClarificationPanelSizes(sizes)) return;
    cache.set(bundleKey, sizes);
    while (cache.size > MAX_CACHED_CLARIFICATION_SIZES) {
      cache.delete(cache.keys().next().value!);
    }
  }, [bundleKey, cache, sizes]);

  const setDesktopSize: Dispatch<SetStateAction<ClarificationPanelSize>> = useCallback((value) => {
    setSizes((current) => ({
      ...current,
      desktop: typeof value === "function" ? value(current.desktop) : value,
    }));
  }, []);
  const setPhoneSize: Dispatch<SetStateAction<ClarificationPanelSize>> = useCallback((value) => {
    setSizes((current) => ({
      ...current,
      phone: typeof value === "function" ? value(current.phone) : value,
    }));
  }, []);

  return {
    desktop: sizes.desktop,
    phone: sizes.phone,
    setDesktopSize,
    setPhoneSize,
  };
}

function clampIndex(index: number, questionCount: number): number {
  if (questionCount <= 0) return 0;
  return Math.max(0, Math.min(index, questionCount - 1));
}

// Until every announced question has arrived, answers for questions that are
// not in the current message list are retained. They restore once the bundle
// is complete instead of being filtered out and written back to the cache.
function isBundleComplete(
  questions: readonly ClarificationDraftQuestion[],
  questionTotal: number | undefined,
): boolean {
  return questionTotal === undefined || questions.length >= questionTotal;
}

function bundleLength(
  questions: readonly ClarificationDraftQuestion[],
  questionTotal: number | undefined,
): number {
  return Math.max(questions.length, questionTotal ?? 0);
}

function restoreAnswers(
  answers: readonly ClarificationAnswer[],
  questions: readonly ClarificationDraftQuestion[],
  complete: boolean,
): readonly ClarificationAnswer[] {
  const optionIdsByQuestion = new Map(
    questions.map((question) => [question.id, question.optionIds]),
  );
  return answers.filter((answer) => {
    const optionIds = optionIdsByQuestion.get(answer.question_id);
    if (!optionIds) return !complete;
    return (answer.selected_options ?? []).every((optionId) => optionIds.includes(optionId));
  });
}

function restoreCustomDrafts(
  customDrafts: Record<string, string>,
  questions: readonly ClarificationDraftQuestion[],
  complete: boolean,
): Record<string, string> {
  const questionIds = new Set(questions.map((question) => question.id));
  const restored: Record<string, string> = {};
  for (const [questionId, text] of Object.entries(customDrafts)) {
    if (questionIds.has(questionId) || !complete) restored[questionId] = text;
  }
  return restored;
}

function restoreDraft(
  draft: RetainedAnswerDraft,
  questions: readonly ClarificationDraftQuestion[] | undefined,
  questionTotal: number | undefined,
): RetainedAnswerDraft {
  if (!questions) return draft;
  const complete = isBundleComplete(questions, questionTotal);
  return {
    customDrafts: restoreCustomDrafts(draft.customDrafts, questions, complete),
    answers: restoreAnswers(draft.answers, questions, complete),
    activeIndex: clampIndex(
      draft.activeIndex,
      complete ? questions.length : bundleLength(questions, questionTotal),
    ),
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
  questionTotal: number | undefined,
): { customDrafts: Record<string, string>; activeIndex: number } {
  if (!questions) return { customDrafts: state.customDrafts, activeIndex: state.activeIndex };
  const complete = isBundleComplete(questions, questionTotal);
  return {
    customDrafts: complete
      ? restoreCustomDrafts(state.customDrafts, questions, true)
      : state.customDrafts,
    activeIndex: clampIndex(
      state.activeIndex,
      complete ? questions.length : bundleLength(questions, questionTotal),
    ),
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
  questions: readonly ClarificationDraftQuestion[] | undefined,
  questionTotal: number | undefined,
): readonly ClarificationAnswer[] {
  const liveAnswers = Object.values(answers);
  let base: readonly ClarificationAnswer[];
  if (state.staleAnswers !== answers && liveAnswers.length > 0) {
    base = liveAnswers;
  } else {
    base = consumedAnswers ? [] : state.pendingAnswers;
  }
  if (!questions || isBundleComplete(questions, questionTotal)) return base;
  const known = new Set(questions.map((question) => question.id));
  const retained = state.pendingAnswers.filter(
    (answer) =>
      !known.has(answer.question_id) &&
      !base.some((candidate) => candidate.question_id === answer.question_id),
  );
  return retained.length > 0 ? [...base, ...retained] : base;
}

function hasMeaningfulCustomText(customDrafts: Record<string, string>): boolean {
  return Object.values(customDrafts).some((text) => text.trim().length > 0);
}

function draftWriteContext(
  context: DraftContextValue | null,
  bundleKey: string | null,
  mode: "active" | "late" | undefined,
) {
  const identity =
    context?.taskId && bundleKey && context.retain && (mode ?? "active") === "active"
      ? answerDraftIdentity(context.taskId, bundleKey)
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
  options?: ClarificationDraftOptions,
) {
  const context = useContext(DraftContext);
  const questions = options?.questions;
  const questionTotal = options?.questionTotal;
  const { identity, scopeKey, owner, generation } = draftWriteContext(
    context,
    bundleKey,
    options?.mode,
  );
  const saved = useMemo(
    () => (identity && owner ? readAnswerDraft(owner, identity) : undefined),
    [identity, owner, generation],
  );
  const restored = useMemo(
    () => (saved ? restoreDraft(saved, questions, questionTotal) : undefined),
    [saved, questions, questionTotal],
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
  const answersToPersist = persistedAnswers(
    state,
    answers,
    consumedAnswers,
    questions,
    questionTotal,
  );
  const { customDrafts, activeIndex } = visibleDraft(state, questions, questionTotal);
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
