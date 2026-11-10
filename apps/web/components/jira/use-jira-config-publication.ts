import { useCallback, useEffect, useRef } from "react";

// Reads share the mounted workspace's sequence; write ACKs fence pending reads.
export function useJiraConfigPublication(workspaceId: string) {
  const currentOwner = useRef<{ sequence: number } | null>(null);
  useEffect(() => {
    currentOwner.current = { sequence: 0 };
    return () => {
      currentOwner.current = null;
    };
  }, [workspaceId]);

  const beginRead = useCallback(() => {
    const owner = currentOwner.current;
    const sequence = owner ? ++owner.sequence : 0;
    const isActive = () => owner !== null && owner === currentOwner.current;
    return {
      isActive,
      isCurrent: () => isActive() && owner?.sequence === sequence,
    };
  }, []);

  const beginWrite = useCallback(() => {
    const owner = currentOwner.current;
    return () => {
      if (!owner || owner !== currentOwner.current) return false;
      ++owner.sequence;
      return true;
    };
  }, []);

  return { beginRead, beginWrite };
}
