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
  onMenuOpenChange,
}: {
  taskPR: TaskPR;
  onMerged?: () => void;
  compact?: boolean;
  /** Bubbles the merge-method dropdown's open state so a host hover-popover
   *  can stay open while the (portaled) menu is active. */
  onMenuOpenChange?: (open: boolean) => void;
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
      onMenuOpenChange={onMenuOpenChange}
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
  onMenuOpenChange,
}: {
  compact: boolean;
  label: string;
  disabled: boolean;
  onPrimaryClick: (e: React.MouseEvent) => void;
  extraMethods: MergeMethod[];
  onPickMethod: (m: MergeMethod) => void;
  onMenuOpenChange?: (open: boolean) => void;
}) {
  // Compact (hover popover): a single full-width primary button plus quiet
  // "or <method>" links for the alternates. Deliberately avoids a dropdown —
  // its menu renders in a detached portal that closes the hover popover the
  // moment the pointer leaves it (the bug this replaces). Plain buttons keep
  // every action inside the popover's own DOM.
  if (compact) {
    return (
      <div className="flex flex-col gap-1.5">
        <button
          type="button"
          data-testid="pr-merge-button"
          onClick={onPrimaryClick}
          disabled={disabled}
          className="inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-green-600 px-2 py-1.5 text-xs font-medium text-white shadow-sm transition-colors hover:bg-green-700 disabled:cursor-default disabled:opacity-60 cursor-pointer"
        >
          <IconGitMerge className="h-3.5 w-3.5" />
          {label}
        </button>
        {extraMethods.length > 0 && (
          <div className="flex items-center justify-center gap-1 text-[11px] text-muted-foreground">
            <span>or</span>
            {extraMethods.map((m) => (
              <button
                key={m}
                type="button"
                data-testid={`pr-merge-method-${m}`}
                disabled={disabled}
                onClick={(e) => {
                  e.stopPropagation();
                  onPickMethod(m);
                }}
                className="cursor-pointer rounded px-1.5 py-0.5 font-medium hover:bg-muted hover:text-foreground disabled:opacity-60"
              >
                {mergeShortLabel(m)}
              </button>
            ))}
          </div>
        )}
      </div>
    );
  }

  const showDropdown = extraMethods.length > 0;
  const primaryBtn = (
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
    return <span className="self-end">{primaryBtn}</span>;
  }

  return (
    <span className="self-end inline-flex">
      {primaryBtn}
      <DropdownMenu onOpenChange={onMenuOpenChange}>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            data-testid="pr-merge-button-more"
            aria-label="Choose merge method"
            disabled={disabled}
            onClick={(e) => e.stopPropagation()}
            className="inline-flex items-center rounded-r-md border-l border-green-700/40 bg-green-600 px-2 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 disabled:opacity-60 cursor-pointer"
          >
            <IconChevronDown className="h-3.5 w-3.5" />
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

// Short form used by the compact "or <method>" alternates so the row fits the
// narrow popover without wrapping.
function mergeShortLabel(method: MergeMethod): string {
  switch (method) {
    case "squash":
      return "Squash";
    case "rebase":
      return "Rebase";
    case "merge":
      return "Merge commit";
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
