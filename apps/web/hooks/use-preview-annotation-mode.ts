"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { bridgeModeForTool, type PreviewAnnotationTool } from "@/lib/preview-annotation";
import { sendSetAnnotationActive, sendSetPreviewCaptureMode } from "@/lib/preview-inspect-bridge";

type Options = {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  enabled: boolean;
  /** The screenshot limit is reached, so the Region tool cannot capture. */
  regionBlocked: boolean;
};

/** Persistent annotation mode for one preview: active flag and the selected tool. */
export function usePreviewAnnotationMode({ iframeRef, enabled, regionBlocked }: Options) {
  const [active, setActive] = useState(false);
  const [tool, setToolState] = useState<PreviewAnnotationTool>("region");
  const bridgeMode = active ? bridgeModeForTool(tool, regionBlocked) : null;
  const stateRef = useRef({ active, bridgeMode });
  stateRef.current = { active, bridgeMode };

  const resend = useCallback(() => {
    const iframe = iframeRef.current;
    if (!iframe) return;
    sendSetPreviewCaptureMode(iframe, stateRef.current.bridgeMode);
    sendSetAnnotationActive(iframe, stateRef.current.active);
  }, [iframeRef]);

  useEffect(resend, [resend, active, bridgeMode]);

  useEffect(() => {
    if (!enabled) setActive(false);
  }, [enabled]);

  const enter = useCallback(() => {
    if (enabled) setActive(true);
  }, [enabled]);
  const exit = useCallback(() => setActive(false), []);
  const toggle = useCallback(() => {
    if (enabled) setActive((current) => !current);
  }, [enabled]);
  const setTool = useCallback((next: PreviewAnnotationTool) => setToolState(next), []);

  return { active, tool, bridgeMode, enter, exit, toggle, setTool, resend };
}
