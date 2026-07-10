"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export function useTooltipMountGate() {
  const [tooltipOpenState, setTooltipOpenState] = useState(false);
  const canOpenTooltipRef = useRef(false);

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      canOpenTooltipRef.current = true;
    });
    return () => cancelAnimationFrame(frame);
  }, []);

  const handleTooltipOpenChange = useCallback((next: boolean) => {
    if (next && !canOpenTooltipRef.current) return;
    setTooltipOpenState(next);
  }, []);

  const closeTooltip = useCallback(() => setTooltipOpenState(false), []);

  return {
    tooltipOpenState,
    handleTooltipOpenChange,
    closeTooltip,
  };
}
