"use client";

import { useCallback, useEffect, useRef, type RefObject } from "react";

export function useDialogOpenerFocus({
  open,
  opener,
  openerRef,
}: {
  open: boolean;
  opener?: HTMLElement | null;
  openerRef?: RefObject<HTMLButtonElement | null>;
}) {
  const capturedOpenerRef = useRef<HTMLElement | null>(null);
  const shouldRestoreFocusRef = useRef(false);
  if (open && opener) capturedOpenerRef.current = opener;

  const requestFocusRestoration = useCallback(() => {
    shouldRestoreFocusRef.current = true;
  }, []);
  const restoreOpenerFocus = useCallback(
    (event?: { preventDefault(): void }) => {
      if (!shouldRestoreFocusRef.current) return;
      const focusTarget = openerRef?.current ?? capturedOpenerRef.current;
      event?.preventDefault();
      shouldRestoreFocusRef.current = false;
      capturedOpenerRef.current = null;
      if (focusTarget?.isConnected && !focusTarget.matches(":disabled")) focusTarget.focus();
    },
    [openerRef],
  );
  useEffect(() => {
    if (!open) restoreOpenerFocus();
  }, [open, restoreOpenerFocus]);

  return { requestFocusRestoration, restoreOpenerFocus };
}
