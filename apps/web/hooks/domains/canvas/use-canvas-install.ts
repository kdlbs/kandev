import { useCallback, useRef, useState } from "react";
import {
  cancelCanvasInstall,
  confirmCanvasInstall,
  prepareCanvasInstall,
  type InstallRequest,
  type InstallResult,
  type InstallReview,
  uploadCanvasInstall,
} from "@/lib/api/domains/canvas-distribution-api";

// eslint-disable-next-line max-lines-per-function -- Preparation, invalidation, and confirmation share one generation guard.
export function useCanvasInstall(workspaceId: string) {
  const [review, setReview] = useState<InstallReview | null>(null);
  const [result, setResult] = useState<InstallResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const generation = useRef(0);
  const reviewRef = useRef<InstallReview | null>(null);

  const updateReview = useCallback((next: InstallReview | null) => {
    reviewRef.current = next;
    setReview(next);
  }, []);

  const begin = useCallback(
    async (operation: () => Promise<InstallReview>) => {
      const current = ++generation.current;
      setLoading(true);
      setError(null);
      updateReview(null);
      setResult(null);
      try {
        const next = await operation();
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
    [updateReview],
  );

  const prepareUrl = useCallback(
    (bundleUrl: string) =>
      begin(() =>
        prepareCanvasInstall({
          workspace_id: workspaceId,
          origin_kind: "url",
          bundle_url: bundleUrl,
        }),
      ),
    [begin, workspaceId],
  );

  const prepareCatalog = useCallback(
    (request: Omit<InstallRequest, "workspace_id" | "bundle_url" | "origin_kind">) =>
      begin(() =>
        prepareCanvasInstall({ ...request, workspace_id: workspaceId, origin_kind: "registry" }),
      ),
    [begin, workspaceId],
  );

  const prepareUpload = useCallback(
    (file: File) =>
      begin(() => uploadCanvasInstall(file, { workspace_id: workspaceId, origin_kind: "upload" })),
    [begin, workspaceId],
  );

  const confirm = useCallback(async () => {
    if (!review) return null;
    const current = generation.current;
    setLoading(true);
    setError(null);
    try {
      const next = await confirmCanvasInstall(
        review.preparation_id,
        review.archive_sha256 ?? review.sha256,
      );
      if (generation.current === current) setResult(next);
      return next;
    } catch (reason) {
      if (generation.current === current) setError(reason);
      throw reason;
    } finally {
      if (generation.current === current) setLoading(false);
    }
  }, [review]);

  const cancel = useCallback(async () => {
    generation.current += 1;
    const currentReview = reviewRef.current;
    if (currentReview) {
      await cancelCanvasInstall(currentReview.preparation_id).catch(() => undefined);
    }
    updateReview(null);
    setResult(null);
    setError(null);
    setLoading(false);
  }, [updateReview]);

  const invalidate = useCallback(() => {
    generation.current += 1;
    const currentReview = reviewRef.current;
    reviewRef.current = null;
    setReview(null);
    setResult(null);
    setError(null);
    setLoading(false);
    if (currentReview) {
      void cancelCanvasInstall(currentReview.preparation_id).catch(() => undefined);
    }
  }, []);

  const reset = useCallback(() => {
    generation.current += 1;
    updateReview(null);
    setResult(null);
    setError(null);
    setLoading(false);
  }, [updateReview]);

  return {
    review,
    result,
    loading,
    error,
    prepareUrl,
    prepareCatalog,
    prepareUpload,
    confirm,
    cancel,
    reset,
    invalidate,
  };
}
