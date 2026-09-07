"use client";

import { useEffect, useRef, useState } from "react";

export type SavedTaskViewDeleteTarget = {
  id: string;
  label: string;
};

export function useSavedTaskViewDeleteConfirmation<T extends HTMLElement = HTMLElement>(
  availableTargets: ReadonlyArray<{ id: string }>,
) {
  const [requestedTarget, setRequestedTarget] = useState<SavedTaskViewDeleteTarget | null>(null);
  const anchorRef = useRef<T>(null);
  const anchorIdRef = useRef<string | null>(null);
  const anchorsRef = useRef(new Map<string, T>());
  const target =
    requestedTarget && availableTargets.some((item) => item.id === requestedTarget.id)
      ? requestedTarget
      : null;

  useEffect(() => {
    if (requestedTarget && !target) setRequestedTarget(null);
  }, [requestedTarget, target]);

  function registerAnchor(id: string, element: T | null) {
    if (element) {
      anchorsRef.current.set(id, element);
      if (anchorIdRef.current === id) anchorRef.current = element;
      return;
    }
    anchorsRef.current.delete(id);
  }

  function request(targetToDelete: SavedTaskViewDeleteTarget) {
    anchorIdRef.current = targetToDelete.id;
    const registeredAnchor = anchorsRef.current.get(targetToDelete.id);
    // Preserve a directly bound anchor when a surface does not register keyed anchors.
    if (registeredAnchor) anchorRef.current = registeredAnchor;
    setRequestedTarget(targetToDelete);
  }

  return {
    target,
    anchorRef,
    close: () => setRequestedTarget(null),
    registerAnchor,
    request,
  };
}
