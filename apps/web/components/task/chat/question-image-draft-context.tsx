import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { ClarificationAnswer } from "@/lib/types/http";

type Draft = {
  customDrafts: Record<string, string>;
  activeIndex: number;
  answers: readonly ClarificationAnswer[];
};
const DraftContext = createContext<Map<string, Draft> | null>(null);

// The task page owns transient drafts across its desktop/phone layout remounts.
// This cache contains no image bytes and does not survive page navigation or reload.
export function QuestionImageDraftProvider({
  children,
  taskId,
}: {
  children: ReactNode;
  taskId: string | null;
}) {
  const [cache, setCache] = useState(() => ({ taskId, drafts: new Map<string, Draft>() }));
  if (cache.taskId !== taskId) setCache({ taskId, drafts: new Map<string, Draft>() });
  return <DraftContext.Provider value={cache.drafts}>{children}</DraftContext.Provider>;
}

export function useQuestionImageDraft(
  bundleKey: string | null,
  retain: boolean,
  answers: Record<string, ClarificationAnswer>,
  settled: boolean,
) {
  const cache = useContext(DraftContext);
  const saved = useMemo(
    () => (retain && bundleKey ? cache?.get(bundleKey) : undefined),
    [cache, bundleKey, retain],
  );
  const [customDrafts, setCustomDrafts] = useState(saved?.customDrafts ?? {});
  const [rawActiveIndex, setActiveIndex] = useState(saved?.activeIndex ?? 0);
  const lastKey = useRef(bundleKey);
  useEffect(() => {
    setCustomDrafts(saved?.customDrafts ?? {});
    setActiveIndex(saved?.activeIndex ?? 0);
  }, [cache, bundleKey, saved]);
  useEffect(() => {
    if (lastKey.current !== bundleKey) {
      lastKey.current = bundleKey;
      return;
    }
    if (!cache || !bundleKey || !retain) return;
    if (settled) {
      cache.delete(bundleKey);
      return;
    }
    cache.set(bundleKey, {
      customDrafts,
      activeIndex: rawActiveIndex,
      answers: Object.values(answers),
    });
    while (cache.size > 8) cache.delete(cache.keys().next().value!);
  }, [cache, bundleKey, retain, settled, customDrafts, rawActiveIndex, answers]);
  return {
    customDrafts,
    setCustomDrafts,
    rawActiveIndex,
    setActiveIndex,
    savedAnswers: saved?.answers,
  };
}
