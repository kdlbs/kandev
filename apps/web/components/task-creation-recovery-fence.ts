"use client";

import { useCallback, useEffect, useRef } from "react";

export type RecoveryOwnerKey = {
  enabled: boolean;
  open: boolean;
  userId: string | null;
  workspaceId: string | null;
  draftId: string;
};

/** Increments on every close, reopen, or scope change to fence late results. */
export function useRecoveryFence({
  enabled,
  open,
  userId,
  workspaceId,
  draftId,
}: RecoveryOwnerKey) {
  const ownerKey = `${enabled}:${open}:${userId ?? ""}:${workspaceId ?? ""}:${draftId}`;
  const ownerKeyRef = useRef(ownerKey);
  const generationRef = useRef(0);
  if (ownerKeyRef.current !== ownerKey) {
    ownerKeyRef.current = ownerKey;
    generationRef.current += 1;
  }

  useEffect(() => {
    return () => {
      generationRef.current += 1;
    };
  }, []);

  const getGeneration = useCallback(() => generationRef.current, []);
  const isOwnerCurrent = useCallback(
    (generation: number) => generation === generationRef.current && open && enabled,
    [enabled, open],
  );
  return { getGeneration, isOwnerCurrent };
}

export type RecoveryFence = ReturnType<typeof useRecoveryFence>;
