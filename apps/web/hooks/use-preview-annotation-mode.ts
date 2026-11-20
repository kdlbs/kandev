"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { bridgeModeForTool, type PreviewAnnotationTool } from "@/lib/preview-annotation";
import { sendSetAnnotationActive, sendSetPreviewCaptureMode } from "@/lib/preview-inspect-bridge";

type Options = {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  enabled: boolean;
  /** The screenshot limit is reached, so the Region tool cannot capture. */
  regionBlocked: boolean;
  /** A text or element comment is open; the tool stays selected but stops picking. */
  paused?: boolean;
  /** A floating comment is open; page presses close it. */
  surfaceOpen?: boolean;
};

/** Persistent annotation mode for one preview: active flag and the selected tool. */
export function usePreviewAnnotationMode({
  iframeRef,
  enabled,
  regionBlocked,
  paused = false,
  surfaceOpen = false,
}: Options) {
  const [active, setActive] = useState(false);
  const [tool, setToolState] = useState<PreviewAnnotationTool>("region");
  const bridgeMode = active && !paused ? bridgeModeForTool(tool, regionBlocked) : null;
  const stateRef = useRef({ active, bridgeMode, surfaceOpen });
  stateRef.current = { active, bridgeMode, surfaceOpen };

  const resend = useCallback(() => {
    const iframe = iframeRef.current;
    if (!iframe) return;
    sendSetPreviewCaptureMode(iframe, stateRef.current.bridgeMode);
    sendSetAnnotationActive(iframe, stateRef.current.active, stateRef.current.surfaceOpen);
  }, [iframeRef]);

  useEffect(resend, [resend, active, bridgeMode, surfaceOpen]);

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
