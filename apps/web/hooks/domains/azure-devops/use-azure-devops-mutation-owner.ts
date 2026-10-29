import { useCallback, useLayoutEffect, useMemo, useRef } from "react";

/** Capture the committed scope before work; retiring it cannot cancel the remote mutation. */
export function useAzureDevOpsMutationOwner(key: string, enabled = true) {
  const owner = useMemo(() => ({ key, enabled }), [key, enabled]);
  const current = useRef<typeof owner | null>(null);
  useLayoutEffect(() => {
    current.current = owner;
    return () => {
      current.current = null;
    };
  }, [owner]);
  return useCallback(() => () => owner.enabled && current.current === owner, [owner]);
}
