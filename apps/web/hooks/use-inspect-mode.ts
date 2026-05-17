"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  sendToggleInspect,
  sendClearAnnotations,
  sendRemoveMarker,
  isInspectorMessage,
  type Annotation,
} from "@/lib/preview-inspect-bridge";

interface UseInspectModeOptions {
  /**
   * Optional gate that lets the host disable inspect mode without losing the
   * underlying user toggle. Used by the preview panel to suspend inspect when
   * the user switches to the logs view but resume it on return. Defaults to true.
   */
  enabled?: boolean;
}

interface UseInspectModeResult {
  /** Effective inspect state (raw toggle AND `options.enabled`). */
  isInspectMode: boolean;
  annotations: Annotation[];
  toggleInspect: () => void;
  /** Attach to the iframe's `onLoad` to re-arm inspect after iframe refresh. */
  handleIframeLoad: () => void;
  handleRemoveAnnotation: (id: string) => void;
  handleClearAnnotations: () => void;
}

/**
 * Owns the inspect-mode state and postMessage wiring shared by every panel
 * that hosts a previewed iframe (currently `BrowserPanel` and `PreviewPanel`).
 *
 * The hook keeps numbering monotonic via a `useRef` counter so panel labels
 * stay correct across remove + add and across iframe refresh. The script-side
 * marker counter resets on iframe reload; the parent counter does not.
 */
export function useInspectMode(
  iframeRef: React.RefObject<HTMLIFrameElement | null>,
  options: UseInspectModeOptions = {},
): UseInspectModeResult {
  const [isInspectMode, setIsInspectMode] = useState(false);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const nextNumber = useRef(1);
  const effectiveIsInspectMode = isInspectMode && (options.enabled ?? true);

  useEffect(() => {
    if (!iframeRef.current) return;
    sendToggleInspect(iframeRef.current, effectiveIsInspectMode);
  }, [iframeRef, effectiveIsInspectMode]);

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      if (!isInspectorMessage(event.data)) return;
      if (event.data.type === "annotation-added") {
        const number = nextNumber.current++;
        setAnnotations((prev) => [...prev, { ...event.data.payload, number }]);
      } else if (event.data.type === "inspect-exited") {
        setIsInspectMode(false);
      }
    }
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [iframeRef]);

  const handleIframeLoad = useCallback(() => {
    if (iframeRef.current && effectiveIsInspectMode) {
      sendToggleInspect(iframeRef.current, true);
    }
  }, [iframeRef, effectiveIsInspectMode]);

  const handleRemoveAnnotation = useCallback(
    (id: string) => {
      const target = annotations.find((a) => a.id === id);
      if (target && iframeRef.current) sendRemoveMarker(iframeRef.current, target.number);
      setAnnotations((prev) => prev.filter((a) => a.id !== id));
    },
    [annotations, iframeRef],
  );

  const handleClearAnnotations = useCallback(() => {
    setAnnotations([]);
    nextNumber.current = 1;
    if (iframeRef.current) sendClearAnnotations(iframeRef.current);
  }, [iframeRef]);

  const toggleInspect = useCallback(() => setIsInspectMode((v) => !v), []);

  return {
    isInspectMode: effectiveIsInspectMode,
    annotations,
    toggleInspect,
    handleIframeLoad,
    handleRemoveAnnotation,
    handleClearAnnotations,
  };
}
