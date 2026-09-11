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

export function useCanvasInstall(workspaceId: string) {
  const [review, setReview] = useState<InstallReview | null>(null);
  const [result, setResult] = useState<InstallResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const generation = useRef(0);

  const begin = useCallback(async (operation: () => Promise<InstallReview>) => {
    const current = ++generation.current;
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const next = await operation();
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
  }, []);

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
    setLoading(true);
    setError(null);
    try {
      const next = await confirmCanvasInstall(
        review.preparation_id,
        review.archive_sha256 ?? review.sha256,
      );
      setResult(next);
      return next;
    } catch (reason) {
      setError(reason);
      throw reason;
    } finally {
      setLoading(false);
    }
  }, [review]);

  const cancel = useCallback(async () => {
    if (!review) return;
    generation.current += 1;
    await cancelCanvasInstall(review.preparation_id).catch(() => undefined);
    setReview(null);
    setResult(null);
  }, [review]);

  const reset = useCallback(() => {
    generation.current += 1;
    setReview(null);
    setResult(null);
    setError(null);
    setLoading(false);
  }, []);

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
  };
}
