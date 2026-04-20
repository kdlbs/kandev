"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { listReviewWatches, triggerAllReviewWatches } from "@/lib/api/domains/github-api";

export function useRefreshReviews() {
  const [loading, setLoading] = useState(false);
  const { toast } = useToast();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const hasWatches = useAppStore((state) => state.reviewWatches.items.length > 0);
  const watchesLoaded = useAppStore((state) => state.reviewWatches.loaded);
  const setReviewWatches = useAppStore((state) => state.setReviewWatches);
  const fetchedRef = useRef(false);

  useEffect(() => {
    if (!workspaceId || watchesLoaded || fetchedRef.current) return;
    fetchedRef.current = true;
    listReviewWatches(workspaceId)
      .then((r) => setReviewWatches(r?.watches ?? []))
      .catch(() => {});
  }, [workspaceId, watchesLoaded, setReviewWatches]);

  const trigger = useCallback(async () => {
    if (!workspaceId || loading) return;
    setLoading(true);
    try {
      const result = await triggerAllReviewWatches(workspaceId);
      const count = result?.new_prs_found ?? 0;
      if (count > 0) {
        toast({
          description: `Found ${count} new PR${count > 1 ? "s" : ""} to review`,
          variant: "success",
        });
      } else {
        toast({ description: "No new PRs to review" });
      }
    } catch {
      toast({ description: "Failed to check for review PRs", variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, loading, toast]);

  return { available: watchesLoaded && hasWatches, loading, trigger };
}
