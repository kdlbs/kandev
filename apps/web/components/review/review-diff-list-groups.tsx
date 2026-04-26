"use client";

import type { ReviewFile } from "./types";

/**
 * Splits files into per-repository groups, preserving file order within each
 * group. Files with no repository_name fall into a single trailing group with
 * empty name (rendered without a repo header).
 */
export function groupFilesByRepository(
  files: ReviewFile[],
): Array<{ repositoryName: string; files: ReviewFile[] }> {
  const order: string[] = [];
  const buckets = new Map<string, ReviewFile[]>();
  for (const file of files) {
    const name = file.repository_name ?? "";
    if (!buckets.has(name)) {
      buckets.set(name, []);
      order.push(name);
    }
    buckets.get(name)!.push(file);
  }
  return order.map((name) => ({ repositoryName: name, files: buckets.get(name)! }));
}

/**
 * Sticky per-repository section header rendered above each group of files in
 * the changes panel. Only shown when the panel actually has multiple repos
 * (or any single named repo) — see ReviewDiffList for the gating logic.
 */
export function RepoGroupHeader({ name, fileCount }: { name: string; fileCount: number }) {
  // The empty-name group ("uncategorised" — no repository_name on its files)
  // gets a generic label so the user understands what they're looking at.
  const label = name || "Other changes";
  return (
    <div
      className="sticky top-0 z-20 flex items-center gap-2 px-4 py-1.5 bg-muted/40 backdrop-blur-sm border-y border-border/60 text-xs font-medium text-foreground"
      data-testid="changes-repo-header"
    >
      <span className="truncate">{label}</span>
      <span className="text-muted-foreground/70">
        {fileCount} {fileCount === 1 ? "file" : "files"}
      </span>
    </div>
  );
}
