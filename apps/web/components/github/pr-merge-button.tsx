"use client";

import { useEffect, useState } from "react";
import { IconGitMerge } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useToast } from "@/components/toast-provider";
import { mergePR } from "@/lib/api/domains/github-api";
import type { TaskPR } from "@/lib/types/github";
import { isPRReadyToMerge } from "./pr-task-icon";

// Renders nothing unless the PR is fully green (CI success + mergeable +
// approval or no-review-needed). Shared by the PR detail panel header and
// the topbar hover popover so a "ready" PR can be merged from either
// surface. `compact` switches to the smaller pill variant used inside the
// dense popover.
export function PRMergeButton({
  taskPR,
  onMerged,
  compact = false,
}: {
  taskPR: TaskPR;
  onMerged?: () => void;
  compact?: boolean;
}) {
  const { toast } = useToast();
  const [merging, setMerging] = useState(false);
  // After a successful merge we stay hidden until the store catches up to
  // state="merged" — otherwise the button briefly re-enables during the async
  // refresh window and a double-click would hit the GitHub API again.
  const [merged, setMerged] = useState(false);

  // If the same component instance ever renders a different PR (e.g. the user
  // switches the active task while the panel/popover stays mounted), the
  // sticky `merged` flag from a previous merge would hide the button for an
  // unrelated, still-mergeable PR. Reset it whenever the underlying PR id
  // changes.
  useEffect(() => {
    setMerged(false);
  }, [taskPR.id]);

  if (merged || !isPRReadyToMerge(taskPR)) return null;

  const handleMerge = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setMerging(true);
    try {
      await mergePR(taskPR.owner, taskPR.repo, taskPR.pr_number);
      setMerged(true);
      toast({ description: "PR merged", variant: "success" });
      onMerged?.();
    } catch (err) {
      toast({
        title: "Failed to merge",
        description: err instanceof Error ? err.message : "An error occurred",
        variant: "error",
      });
    } finally {
      setMerging(false);
    }
  };

  if (compact) {
    return (
      <button
        type="button"
        data-testid="pr-merge-button"
        onClick={handleMerge}
        disabled={merging}
        className="self-end inline-flex items-center gap-1 rounded-full bg-green-600 px-2 py-0.5 text-[11px] font-medium text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 disabled:opacity-60 cursor-pointer"
      >
        <IconGitMerge className="h-3 w-3" />
        {merging ? "Merging..." : "Merge"}
      </button>
    );
  }

  return (
    <Button
      data-testid="pr-merge-button"
      size="sm"
      className="cursor-pointer gap-1.5 border-0 bg-green-600 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500"
      onClick={handleMerge}
      disabled={merging}
    >
      <IconGitMerge className="h-3.5 w-3.5" />
      {merging ? "Merging..." : "Merge PR"}
    </Button>
  );
}
