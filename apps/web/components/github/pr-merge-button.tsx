"use client";

import { useEffect, useState } from "react";
import { IconChevronDown, IconGitMerge } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useToast } from "@/components/toast-provider";
import { useRepoMergeMethods } from "@/hooks/domains/github/use-repo-merge-methods";
import { mergePR } from "@/lib/api/domains/github-api";
import type { MergeMethod, TaskPR } from "@/lib/types/github";
import { isPRReadyToMerge } from "./pr-task-icon";

// Renders nothing unless the PR is fully green (CI success + mergeable +
// approval or no-review-needed). When `useRepoMergeMethods` hasn't yet
// returned the repo's allowed methods (loading, failure, or expired cache)
// the button still renders — clicks fall through to the backend resolver.
// Shared by the PR detail panel header and the topbar hover popover so a
// "ready" PR can be merged from either surface. `compact` switches to the
// smaller pill variant used inside the dense popover.
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
  const methods = useRepoMergeMethods(taskPR.owner, taskPR.repo);

  // If the same component instance ever renders a different PR (e.g. the user
  // switches the active task while the panel/popover stays mounted), the
  // sticky `merged` flag from a previous merge would hide the button for an
  // unrelated, still-mergeable PR. Reset it whenever the underlying PR id
  // changes.
  useEffect(() => {
    setMerged(false);
  }, [taskPR.id]);

  if (merged || !isPRReadyToMerge(taskPR)) return null;
  // `methods` may be null on first render, on lookup failure, or after the
  // 5-minute cache window. We still render the button — clicking with no
  // method routes through the backend's GetRepoMergeMethods resolver, so
  // the merge succeeds either way. The trade-off is a brief label flicker
  // from "Merge PR" → "Squash and merge" on first load; locking the user
  // out of merging on a transient fetch failure is the worse alternative.
  const allowed = allowedMethods(methods);
  const primary = pickPrimaryMethod(allowed);

  const runMerge = async (method?: MergeMethod) => {
    setMerging(true);
    try {
      await mergePR(taskPR.owner, taskPR.repo, taskPR.pr_number, method);
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

  const handlePrimary = (e: React.MouseEvent) => {
    e.stopPropagation();
    void runMerge(primary);
  };

  return (
    <MergeButtonShell
      compact={compact}
      label={merging ? "Merging..." : mergeLabel(primary)}
      disabled={merging}
      onPrimaryClick={handlePrimary}
      extraMethods={allowed.filter((m) => m !== primary)}
      onPickMethod={(m) => void runMerge(m)}
    />
  );
}

function MergeButtonShell({
  compact,
  label,
  disabled,
  onPrimaryClick,
  extraMethods,
  onPickMethod,
}: {
  compact: boolean;
  label: string;
  disabled: boolean;
  onPrimaryClick: (e: React.MouseEvent) => void;
  extraMethods: MergeMethod[];
  onPickMethod: (m: MergeMethod) => void;
}) {
  const showDropdown = extraMethods.length > 0;
  const primaryBtn = compact ? (
    <button
      type="button"
      data-testid="pr-merge-button"
      onClick={onPrimaryClick}
      disabled={disabled}
      className={`inline-flex items-center gap-1 bg-green-600 px-2 py-0.5 text-[11px] font-medium text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 disabled:opacity-60 cursor-pointer ${showDropdown ? "rounded-l-full" : "rounded-full"}`}
    >
      <IconGitMerge className="h-3 w-3" />
      {label}
    </button>
  ) : (
    <Button
      data-testid="pr-merge-button"
      size="sm"
      className={`cursor-pointer gap-1.5 border-0 bg-green-600 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 ${showDropdown ? "rounded-r-none" : ""}`}
      onClick={onPrimaryClick}
      disabled={disabled}
    >
      <IconGitMerge className="h-3.5 w-3.5" />
      {label}
    </Button>
  );

  if (!showDropdown) {
    return compact ? primaryBtn : <span className="self-end">{primaryBtn}</span>;
  }

  return (
    <span className={compact ? "inline-flex" : "self-end inline-flex"}>
      {primaryBtn}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            data-testid="pr-merge-button-more"
            aria-label="Choose merge method"
            disabled={disabled}
            onClick={(e) => e.stopPropagation()}
            className={
              compact
                ? "inline-flex items-center rounded-r-full border-l border-green-700/40 bg-green-600 px-1.5 py-0.5 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 disabled:opacity-60 cursor-pointer"
                : "inline-flex items-center rounded-r-md border-l border-green-700/40 bg-green-600 px-2 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 disabled:opacity-60 cursor-pointer"
            }
          >
            <IconChevronDown className={compact ? "h-3 w-3" : "h-3.5 w-3.5"} />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-auto">
          {extraMethods.map((m) => (
            <DropdownMenuItem key={m} onSelect={() => onPickMethod(m)}>
              {mergeLabel(m)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </span>
  );
}

function mergeLabel(method?: MergeMethod): string {
  switch (method) {
    case "squash":
      return "Squash and merge";
    case "rebase":
      return "Rebase and merge";
    case "merge":
      return "Create a merge commit";
    default:
      return "Merge PR";
  }
}

// Returns the methods the repo allows, in the order we prefer to surface
// them (squash → merge → rebase, matching GitHub's own button defaults).
// When the lookup hasn't resolved (or failed), returns an empty array so
// the button still renders without a dropdown and the backend's resolver
// picks the method at merge time.
function allowedMethods(
  methods: { merge: boolean; squash: boolean; rebase: boolean } | null,
): MergeMethod[] {
  if (!methods) return [];
  const order: MergeMethod[] = ["squash", "merge", "rebase"];
  return order.filter((m) => methods[m]);
}

function pickPrimaryMethod(allowed: MergeMethod[]): MergeMethod | undefined {
  return allowed[0];
}
