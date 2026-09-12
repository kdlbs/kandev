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
  const reviewRef = useRef<ExportReview | null>(null);

  const updateReview = useCallback((next: ExportReview | null) => {
    reviewRef.current = next;
    setReview(next);
  }, []);

  const prepare = useCallback(
    async (metadata?: DistributionMetadata) => {
      if (!canvas?.active_release_id) return null;
      const current = ++generation.current;
      setLoading(true);
      setError(null);
      updateReview(null);
      try {
        const next = await prepareCanvasExport(canvas.id, {
          workspace_id: canvas.workspace_id,
          expected_release_id: canvas.active_release_id,
          metadata,
        });
        if (generation.current === current) updateReview(next);
        return next;
      } catch (reason) {
        if (generation.current === current) {
          updateReview(null);
          setError(reason);
        }
        throw reason;
      } finally {
        if (generation.current === current) setLoading(false);
      }
    },
    [canvas, updateReview],
  );

  const download = useCallback(
    async (kind: "bundle" | "source") => {
      if (!review) return;
      const current = ++generation.current;
      setLoading(true);
      setError(null);
      try {
        const blob = await downloadCanvasExport(review.preparation_id, kind);
        const packageId = review.metadata.package_id ?? review.canvas_id;
        const version = review.metadata.version ?? "release";
        if (generation.current === current) {
          triggerBlobDownload(
            blob,
            `${packageId}-${version}.${kind === "bundle" ? "tar.gz" : "zip"}`,
          );
        }
      } catch (reason) {
        if (generation.current === current) setError(reason);
        throw reason;
      } finally {
        if (generation.current === current) setLoading(false);
      }
    },
    [review],
  );

  const cancel = useCallback(async () => {
    generation.current += 1;
    const currentReview = reviewRef.current;
    if (currentReview) {
      await cancelCanvasExport(currentReview.preparation_id).catch(() => undefined);
    }
    updateReview(null);
    setError(null);
  }, [updateReview]);

  const invalidate = useCallback(() => {
    generation.current += 1;
    const currentReview = reviewRef.current;
    reviewRef.current = null;
    setReview(null);
    setError(null);
    setLoading(false);
    if (currentReview) {
      void cancelCanvasExport(currentReview.preparation_id).catch(() => undefined);
    }
  }, []);

  const reset = useCallback(() => {
    generation.current += 1;
    updateReview(null);
    setError(null);
    setLoading(false);
  }, [updateReview]);

  return { review, loading, error, prepare, download, cancel, reset, invalidate };
}
