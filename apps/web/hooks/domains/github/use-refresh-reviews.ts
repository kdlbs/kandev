"use client";

import { useCallback, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useReviewWatches } from "./use-review-watches";

export function useRefreshReviews() {
  const [loading, setLoading] = useState(false);
  const { toast } = useToast();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { items, loaded, triggerAll } = useReviewWatches(workspaceId ?? null);

  const trigger = useCallback(async () => {
    if (loading) return;
    setLoading(true);
    try {
      const result = await triggerAll();
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
  }, [loading, triggerAll, toast]);

  return { available: loaded && items.length > 0, loading, trigger };
}
