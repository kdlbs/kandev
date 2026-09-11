import { useCallback, useRef, useState } from "react";
import {
  cancelCanvasExport,
  downloadCanvasExport,
  prepareCanvasExport,
  type DistributionMetadata,
  type ExportReview,
} from "@/lib/api/domains/canvas-distribution-api";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { triggerBlobDownload } from "@/lib/utils/file-download";

export function useCanvasShare(canvas: Canvas | null) {
  const [review, setReview] = useState<ExportReview | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const generation = useRef(0);

  const prepare = useCallback(
    async (metadata?: DistributionMetadata) => {
      if (!canvas?.active_release_id) return null;
      const current = ++generation.current;
      setLoading(true);
      setError(null);
      try {
        const next = await prepareCanvasExport(canvas.id, {
          workspace_id: canvas.workspace_id,
          expected_release_id: canvas.active_release_id,
          metadata,
        });
        if (generation.current === current) setReview(next);
        return next;
      } catch (reason) {
        if (generation.current === current) {
          setReview(null);
          setError(reason);
        }
        throw reason;
      } finally {
        if (generation.current === current) setLoading(false);
      }
    },
    [canvas],
  );

  const download = useCallback(
    async (kind: "bundle" | "source") => {
      if (!review) return;
      setLoading(true);
      setError(null);
      try {
        const blob = await downloadCanvasExport(review.preparation_id, kind);
        const packageId = review.metadata.package_id ?? review.canvas_id;
        const version = review.metadata.version ?? "release";
        triggerBlobDownload(
          blob,
          `${packageId}-${version}.${kind === "bundle" ? "tar.gz" : "zip"}`,
        );
      } catch (reason) {
        setError(reason);
        throw reason;
      } finally {
        setLoading(false);
      }
    },
    [review],
  );

  const cancel = useCallback(async () => {
    generation.current += 1;
    if (review) await cancelCanvasExport(review.preparation_id).catch(() => undefined);
    setReview(null);
    setError(null);
  }, [review]);

  const reset = useCallback(() => {
    generation.current += 1;
    setReview(null);
    setError(null);
    setLoading(false);
  }, []);

  return { review, loading, error, prepare, download, cancel, reset };
}
