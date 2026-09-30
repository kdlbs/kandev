"use client";

import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import type { VisibleRow } from "@/hooks/use-tree";
import type { ChangesTreeNode } from "./changes-file-tree-model";

export function TreeDirRow({
  row,
  baseIndentPx,
  onToggle,
  testId,
}: {
  row: VisibleRow<ChangesTreeNode>;
  baseIndentPx: number;
  onToggle: () => void;
  testId?: string;
}) {
  return (
    <button
      type="button"
      className="flex min-h-11 w-full items-center gap-1 rounded-md px-1 py-0.5 -mx-1 text-left text-xs text-foreground/70 hover:bg-muted/60 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      style={{ paddingLeft: baseIndentPx + row.depth * 12 + 4 }}
      onClick={onToggle}
      aria-expanded={row.isExpanded}
      data-testid={testId ?? `tree-dir-${row.path.replace(/[/\\]/g, "-")}`}
      data-changes-tree-directory={row.path}
      data-changes-row-focus
    >
      {row.isExpanded ? (
        <IconChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
      ) : (
        <IconChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
      )}
      <span className="truncate">{row.displayName}</span>
    </button>
  );
}
