"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { triggerAllReviewWatches, listReviewWatches } from "@/lib/api/domains/github-api";

export function RefreshReviewsButton() {
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

  const handleTrigger = useCallback(async () => {
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

  if (!watchesLoaded || !hasWatches) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          className="cursor-pointer"
          onClick={handleTrigger}
          disabled={loading}
        >
          <IconRefresh className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Check for PRs to review</TooltipContent>
    </Tooltip>
  );
}
