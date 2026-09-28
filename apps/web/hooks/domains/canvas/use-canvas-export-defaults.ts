import { useCallback, useEffect, useState } from "react";
import {
  getCanvasExportDefaults,
  type ExportDefaults,
} from "@/lib/api/domains/canvas-distribution-api";

export function useCanvasExportDefaults(
  canvasId: string | undefined,
  releaseId: string | undefined,
  enabled: boolean,
) {
  const [defaults, setDefaults] = useState<ExportDefaults | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [retryKey, setRetryKey] = useState(0);
  const retry = useCallback(() => setRetryKey((value) => value + 1), []);

  useEffect(() => {
    setDefaults(null);
    setError(false);
    if (!enabled || !canvasId || !releaseId) {
      setLoading(false);
      return;
    }
    let current = true;
    const controller = new AbortController();
    setLoading(true);
    getCanvasExportDefaults(canvasId, { cache: "no-store", init: { signal: controller.signal } })
      .then((value) => {
        if (!current) return;
        if (value.expected_release_id !== releaseId) throw new Error("stale release");
        setDefaults(value);
      })
      .catch(() => {
        if (current) setError(true);
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [canvasId, releaseId, enabled, retryKey]);

  const currentDefaults = enabled && defaults?.expected_release_id === releaseId ? defaults : null;
  return { defaults: currentDefaults, loading, error, retry };
}
