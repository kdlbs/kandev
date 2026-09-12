import { useEffect, type RefObject } from "react";

export type TranscriptViewportOptions = {
  scrollRef: RefObject<HTMLDivElement | null>;
  sessionId: string | null;
  enabled: boolean;
  isVisible: boolean;
  initialPlacementPending: boolean;
  isProgrammaticScrollLocked: () => boolean;
};

function readViewport(element: HTMLElement) {
  return {
    width: element.clientWidth,
    height: element.clientHeight,
    atBottom: element.scrollHeight - element.scrollTop - element.clientHeight < 3,
  };
}

/** Footer allocation changes must not turn a bottom-following reader into a history reader. */
export function useTranscriptViewportResize({
  scrollRef,
  sessionId,
  enabled,
  isVisible,
  initialPlacementPending,
  isProgrammaticScrollLocked,
}: TranscriptViewportOptions) {
  useEffect(() => {
    const element = scrollRef.current;
    if (!element || !enabled || !isVisible || initialPlacementPending) return;
    let previous = readViewport(element);
    const recordScroll = () => {
      const current = readViewport(element);
      // Native clamping may emit scroll before the resize observer sees new geometry.
      if (current.width === previous.width && current.height === previous.height)
        previous = current;
    };
    const observer = new ResizeObserver(() => {
      const current = readViewport(element);
      const heightChanged =
        current.height !== previous.height && previous.height > 0 && current.height > 0;
      if (
        heightChanged &&
        current.width === previous.width &&
        previous.atBottom &&
        !isProgrammaticScrollLocked()
      ) {
        element.scrollTop = element.scrollHeight - element.clientHeight;
      }
      previous = readViewport(element);
    });
    observer.observe(element);
    element.addEventListener("scroll", recordScroll, { passive: true });
    return () => {
      observer.disconnect();
      element.removeEventListener("scroll", recordScroll);
    };
  }, [
    scrollRef,
    sessionId,
    enabled,
    isVisible,
    initialPlacementPending,
    isProgrammaticScrollLocked,
  ]);
}
