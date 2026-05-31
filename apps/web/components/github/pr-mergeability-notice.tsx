"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MergeableState } from "@/lib/types/github";

// --- Pure descriptor (unit-tested) ---

export type MergeabilityNotice =
  | { kind: "none" }
  | { kind: "banner" } // dirty / merge conflicts
  | { kind: "chip"; label: string } // blocked / behind base
  | { kind: "text" }; // generic non-mergeable fallback

/** Maps a PR's merge state to how prominently the panel surfaces it. */
export function describeMergeability({
  state,
  mergeable,
  isDraft,
  prState,
}: {
  state: MergeableState | undefined;
  mergeable: boolean;
  isDraft: boolean;
  prState: string;
}): MergeabilityNotice {
  if (prState !== "open" || isDraft) return { kind: "none" };
  switch (state) {
    case "dirty":
      return { kind: "banner" };
    case "blocked":
      return { kind: "chip", label: "Blocked" };
    case "behind":
      return { kind: "chip", label: "Behind base" };
    case "clean":
    case "unstable":
    case "has_hooks":
    case "draft":
      // "draft" is gated above via isDraft, but handle the enum value too.
      return { kind: "none" };
    default:
      // unknown / "" / future states: defer to the legacy boolean signal.
      return mergeable === false ? { kind: "text" } : { kind: "none" };
  }
}

/** Chat-context message sent to the agent when the user clicks "Resolve conflicts". */
export function buildConflictResolutionMessage({
  prNumber,
  headBranch,
  baseBranch,
}: {
  prNumber: number;
  headBranch: string;
  baseBranch: string;
}): string {
  return (
    `PR #${prNumber} (\`${headBranch}\` → \`${baseBranch}\`) has merge conflicts and ` +
    `can't be merged automatically. Please merge \`${baseBranch}\` into \`${headBranch}\` ` +
    `(or rebase onto it) and resolve the conflicts.`
  );
}

// --- Presentation ---

function ConflictBanner({
  baseBranch,
  onResolveConflicts,
  resolveDisabled,
}: {
  baseBranch: string;
  onResolveConflicts?: () => void;
  resolveDisabled?: boolean;
}) {
  return (
    <div
      data-testid="pr-conflict-banner"
      className="flex items-start gap-2.5 rounded-md border border-red-500/35 bg-red-500/10 px-3 py-2.5"
    >
      <IconAlertTriangle className="h-4 w-4 shrink-0 text-red-600 dark:text-red-400 mt-0.5" />
      <div className="min-w-0 flex-1">
        <div className="text-xs font-semibold text-red-600 dark:text-red-400">
          Merge conflicts with <code className="font-mono">{baseBranch}</code>
        </div>
        <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground">
          This branch can&apos;t be merged automatically. Resolve the conflicts before merging.
        </p>
        {onResolveConflicts && (
          <Button
            size="sm"
            variant="outline"
            data-testid="pr-resolve-conflicts-button"
            className="mt-2 h-6 cursor-pointer px-2 text-[11px]"
            onClick={onResolveConflicts}
            disabled={resolveDisabled}
          >
            {resolveDisabled ? "Added to chat context" : "Resolve conflicts"}
          </Button>
        )}
      </div>
    </div>
  );
}

function MergeabilityChip({ label }: { label: string }) {
  return (
    <span className="flex items-center gap-1 text-[10px] text-amber-600 dark:text-amber-400">
      <IconAlertTriangle className="h-3 w-3" />
      {label}
    </span>
  );
}

function NotMergeableText() {
  return (
    <span className="flex items-center gap-1 text-[10px] text-yellow-600 dark:text-yellow-400">
      <IconAlertTriangle className="h-3 w-3" />
      Not mergeable
    </span>
  );
}

export function PRMergeabilityNotice({
  state,
  mergeable,
  isDraft,
  prState,
  baseBranch,
  onResolveConflicts,
  resolveDisabled,
}: {
  state: MergeableState | undefined;
  mergeable: boolean;
  isDraft: boolean;
  prState: string;
  baseBranch: string;
  onResolveConflicts?: () => void;
  resolveDisabled?: boolean;
}) {
  const notice = describeMergeability({ state, mergeable, isDraft, prState });
  if (notice.kind === "none") return null;
  if (notice.kind === "banner")
    return (
      <ConflictBanner
        baseBranch={baseBranch}
        onResolveConflicts={onResolveConflicts}
        resolveDisabled={resolveDisabled}
      />
    );
  return (
    <div className="flex items-center gap-1.5 flex-wrap">
      {notice.kind === "chip" ? <MergeabilityChip label={notice.label} /> : <NotMergeableText />}
    </div>
  );
}
