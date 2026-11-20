"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type RuntimeUpdateManagerView =
  | { kind: "closed" }
  | { kind: "overview" }
  | { kind: "runtime"; agentName: string };

type OpenRuntime = (agentName: string, opener?: HTMLElement | null) => void;

/**
 * Owns the single runtime manager surface and the small amount of state that
 * must outlive the conditional overlay: the current view and the element that
 * opened it. Policy drafts and approved bulk work live in their own controllers.
 */
export function useAgentRuntimeUpdateManager() {
  const [view, setView] = useState<RuntimeUpdateManagerView>({ kind: "closed" });
  const openerRef = useRef<HTMLElement | null>(null);
  const headerButtonRef = useRef<HTMLButtonElement | null>(null);
  const previousKind = useRef<RuntimeUpdateManagerView["kind"]>("closed");

  const captureOpener = useCallback((opener?: HTMLElement | null) => {
    if (opener) {
      openerRef.current = opener;
      return;
    }
    const active = document.activeElement;
    openerRef.current = active instanceof HTMLElement && active !== document.body ? active : null;
  }, []);

  const openOverview = useCallback(
    (opener?: HTMLElement | null) => {
      captureOpener(opener);
      setView({ kind: "overview" });
    },
    [captureOpener],
  );

  const openRuntime = useCallback<OpenRuntime>(
    (agentName, opener) => {
      if (!agentName) return;
      captureOpener(opener);
      setView({ kind: "runtime", agentName });
    },
    [captureOpener],
  );

  const back = useCallback(() => setView({ kind: "overview" }), []);

  const close = useCallback(() => setView({ kind: "closed" }), []);

  const setOpen = useCallback((open: boolean) => {
    if (!open) setView({ kind: "closed" });
  }, []);

  useEffect(() => {
    const wasOpen = previousKind.current !== "closed";
    previousKind.current = view.kind;
    if (!wasOpen || view.kind !== "closed") return;
    const target = openerRef.current;
    const fallback = headerButtonRef.current;
    const focusTarget = target?.isConnected ? target : fallback;
    openerRef.current = null;
    focusTarget?.focus();
  }, [view.kind]);

  return {
    view,
    openOverview,
    openRuntime,
    back,
    close,
    setOpen,
    headerButtonRef,
  };
}
