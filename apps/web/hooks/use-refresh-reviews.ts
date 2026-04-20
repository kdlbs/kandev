"use client";

import { useCallback, useEffect, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { listReviewWatches, triggerAllReviewWatches } from "@/lib/api/domains/github-api";

// Module-scoped so concurrent hook instances share the in-flight guard and
// only one listReviewWatches request fires on mount.
let fetchInFlight = false;

export function useRefreshReviews() {
  const [loading, setLoading] = useState(false);
  const { toast } = useToast();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const hasWatches = useAppStore((state) => state.reviewWatches.items.length > 0);
  const watchesLoaded = useAppStore((state) => state.reviewWatches.loaded);
  const setReviewWatches = useAppStore((state) => state.setReviewWatches);

  useEffect(() => {
    if (!workspaceId || watchesLoaded || fetchInFlight) return;
    fetchInFlight = true;
    listReviewWatches(workspaceId)
      .then((r) => setReviewWatches(r?.watches ?? []))
      .catch(() => {})
      .finally(() => {
        fetchInFlight = false;
      });
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
