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

type Draft = {
  customDrafts: Record<string, string>;
  activeIndex: number;
  answers: readonly ClarificationAnswer[];
};
type ClarificationPanelSize = { desktop: number | null; phone: number | null };
const EMPTY_PANEL_SIZE: ClarificationPanelSize = { desktop: null, phone: null };
const MAX_CACHED_CLARIFICATION_SIZES = 8;
const DraftContext = createContext<Map<string, Draft> | null>(null);
const ResizeContext = createContext<Map<string, ClarificationPanelSize> | null>(null);

// The task page owns transient clarification drafts across desktop/phone remounts.
// The cache contains no image bytes and does not survive navigation or reload.
export function ClarificationDraftProvider({
  children,
  taskId,
}: {
  children: ReactNode;
  taskId: string | null;
}) {
  const [cache, setCache] = useState(() => ({
    taskId,
    drafts: new Map<string, Draft>(),
    sizes: new Map<string, ClarificationPanelSize>(),
  }));
  if (cache.taskId !== taskId) {
    setCache({
      taskId,
      drafts: new Map<string, Draft>(),
      sizes: new Map<string, ClarificationPanelSize>(),
    });
  }
  return (
    <DraftContext.Provider value={cache.drafts}>
      <ResizeContext.Provider value={cache.sizes}>{children}</ResizeContext.Provider>
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

export function useClarificationDraft(
  bundleKey: string | null,
  answers: Record<string, ClarificationAnswer>,
  settled: boolean,
) {
  const cache = useContext(DraftContext);
  const saved = useMemo(() => (bundleKey ? cache?.get(bundleKey) : undefined), [cache, bundleKey]);
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
    if (!cache || !bundleKey) return;
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
  }, [cache, bundleKey, settled, customDrafts, rawActiveIndex, answers]);
  return {
    customDrafts,
    setCustomDrafts,
    rawActiveIndex,
    setActiveIndex,
    savedAnswers: saved?.answers,
  };
}
