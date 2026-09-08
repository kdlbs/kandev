import { useCallback, useEffect, useRef, useState } from "react";

export function usePillTooltipSuppression(open: boolean) {
  const [suppressTooltip, setSuppressTooltip] = useState(false);
  const suppressTooltipRef = useRef(false);
  const pointerInsideTriggerRef = useRef(false);
  const suppressionReleaseOnExitRef = useRef(true);
  const suppressionReleaseArmedRef = useRef(false);
  const suppressionReleaseFrameRef = useRef<number | null>(null);
  const clearTooltipSuppression = useCallback(() => {
    if (suppressionReleaseFrameRef.current !== null) {
      cancelAnimationFrame(suppressionReleaseFrameRef.current);
      suppressionReleaseFrameRef.current = null;
    }
    suppressionReleaseArmedRef.current = false;
    suppressionReleaseOnExitRef.current = true;
    suppressTooltipRef.current = false;
    setSuppressTooltip(false);
  }, []);
  const suppressTooltipUntilLeave = useCallback(
    (releaseOnExit = true) => {
      if (suppressionReleaseFrameRef.current !== null) {
        cancelAnimationFrame(suppressionReleaseFrameRef.current);
      }
      suppressionReleaseOnExitRef.current = releaseOnExit;
      suppressionReleaseArmedRef.current = false;
      suppressTooltipRef.current = true;
      setSuppressTooltip(true);
      suppressionReleaseFrameRef.current = requestAnimationFrame(() => {
        suppressionReleaseFrameRef.current = requestAnimationFrame(() => {
          suppressionReleaseFrameRef.current = null;
          if (!pointerInsideTriggerRef.current && suppressionReleaseOnExitRef.current) {
            clearTooltipSuppression();
            return;
          }
          suppressionReleaseArmedRef.current = true;
        });
      });
    },
    [clearTooltipSuppression],
  );
  useEffect(
    () => () => {
      if (suppressionReleaseFrameRef.current !== null) {
        cancelAnimationFrame(suppressionReleaseFrameRef.current);
      }
    },
    [],
  );
  const handlePointerEnter = useCallback(
    (event: React.PointerEvent) => {
      pointerInsideTriggerRef.current = true;
      if (
        event.pointerType !== "touch" &&
        !suppressionReleaseOnExitRef.current &&
        suppressTooltipRef.current
      ) {
        clearTooltipSuppression();
      }
    },
    [clearTooltipSuppression],
  );
  const handlePointerLeave = useCallback(() => {
    pointerInsideTriggerRef.current = false;
    if (!open && suppressionReleaseOnExitRef.current && suppressionReleaseArmedRef.current) {
      clearTooltipSuppression();
    }
  }, [clearTooltipSuppression, open]);
  const handleBlur = useCallback(() => {
    if (!open && suppressionReleaseOnExitRef.current && suppressionReleaseArmedRef.current) {
      clearTooltipSuppression();
    }
  }, [clearTooltipSuppression, open]);

  return {
    suppressTooltip,
    suppressTooltipRef,
    suppressTooltipUntilLeave,
    handlePointerEnter,
    handlePointerLeave,
    handleBlur,
  };
}
