import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";

export type ConnectionDraftCache = Map<string, unknown>;
export const GitHubConnectionDraftContext = createContext<ConnectionDraftCache | null>(null);

/** Retain temporary App form state while the responsive overlay changes. */
export function useConnectionFormState<T>(
  key: string,
  initial: T,
): [T, Dispatch<SetStateAction<T>>] {
  const cache = useContext(GitHubConnectionDraftContext);
  const [value, setValue] = useState<T>(() => (cache?.has(key) ? (cache.get(key) as T) : initial));
  const update: Dispatch<SetStateAction<T>> = useCallback(
    (next) =>
      setValue((current) => {
        const updated = typeof next === "function" ? (next as (value: T) => T)(current) : next;
        cache?.set(key, updated);
        return updated;
      }),
    [cache, key],
  );
  return [value, update];
}

/** Fence async results when a draft is replaced or its overlay is dismissed. */
export function useConnectionRequestGuard(scope: string) {
  const identity = useRef({ scope });
  const mounted = useRef(true);
  if (identity.current.scope !== scope) identity.current = { scope };
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  return () => {
    const started = identity.current;
    return () => mounted.current && identity.current === started;
  };
}
