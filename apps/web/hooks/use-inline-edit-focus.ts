import { useCallback, useLayoutEffect, useRef } from "react";
import type { FocusEvent, RefObject } from "react";

type EditFocusOptions = {
  active: boolean;
  identity?: unknown;
  inputRef: RefObject<HTMLInputElement | null>;
  containerRef?: RefObject<HTMLElement | null>;
  onLeave: () => void;
};

function observeEditFocus(input: HTMLInputElement, container: HTMLElement, onLeave: () => void) {
  const doc = input.ownerDocument;
  const win = doc.defaultView!;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let finished = false;
  let outsidePointer = false;
  let engaged = doc.activeElement === input;

  const clearPending = () => {
    clearTimeout(timer);
    timer = undefined;
    outsidePointer = false;
  };
  const cancel = () => {
    finished = true;
    clearPending();
  };
  const leave = () => {
    if (finished) return;
    cancel();
    onLeave();
  };
  const contains = (target: EventTarget | null) =>
    target instanceof win.Node && container.contains(target);
  const defer = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      const pointer = outsidePointer;
      clearPending();
      if (finished || doc.visibilityState === "hidden" || !doc.hasFocus()) return;
      const target = doc.activeElement;
      if (pointer || (target && target !== doc.body && !contains(target))) leave();
    }, 0);
  };
  const blur = (target: EventTarget | null) => {
    if (contains(target)) clearPending();
    else if (target instanceof win.Node && target.ownerDocument === doc) leave();
    else defer();
  };
  const pointerDown = (event: PointerEvent) => {
    if (!engaged) return;
    clearPending();
    if (contains(event.target)) return;
    outsidePointer = true;
    defer();
  };
  // A focus event on body is an internal move; body alone in the deferred check is ambiguous.
  const focusIn = (event: globalThis.FocusEvent) => {
    if (contains(event.target)) {
      engaged = true;
      clearPending();
    } else if (engaged) leave();
  };
  doc.addEventListener("pointerdown", pointerDown, true);
  doc.addEventListener("focusin", focusIn);

  return {
    blur,
    cancel,
    dispose: () => {
      cancel();
      doc.removeEventListener("pointerdown", pointerDown, true);
      doc.removeEventListener("focusin", focusIn);
    },
  };
}

/** Keep external blur nonterminal while preserving the editor's internal leave action. */
export function useInlineEditFocus({
  active,
  identity,
  inputRef,
  containerRef,
  onLeave,
}: EditFocusOptions) {
  const leaveRef = useRef(onLeave);
  const observerRef = useRef<ReturnType<typeof observeEditFocus> | null>(null);
  useLayoutEffect(() => {
    leaveRef.current = onLeave;
  });
  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!active || !input) return;
    const observer = observeEditFocus(input, containerRef?.current ?? input, () =>
      leaveRef.current(),
    );
    observerRef.current = observer;
    return () => {
      observer.dispose();
      observerRef.current = null;
    };
  }, [active, identity, inputRef, containerRef]);

  const onBlur = useCallback((event: FocusEvent<HTMLInputElement>) => {
    observerRef.current?.blur(event.relatedTarget);
  }, []);
  const cancelPendingBlur = useCallback(() => observerRef.current?.cancel(), []);
  return { onBlur, cancelPendingBlur };
}
