"use client";

import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import type { VisibleRow } from "@/hooks/use-tree";
import type { ChangedFile } from "./changes-panel-helpers";
import type { ChangesTreeFile, ChangesTreeNode } from "./changes-file-tree-model";
import { cn } from "@/lib/utils";

export function TreeDirRow<TFile extends ChangesTreeFile = ChangedFile>({
  row,
  baseIndentPx,
  onToggle,
  testId,
  touchMode = false,
}: {
  row: VisibleRow<ChangesTreeNode<TFile>>;
  baseIndentPx: number;
  onToggle: () => void;
  testId?: string;
  touchMode?: boolean;
}) {
  const indentPx = baseIndentPx + row.depth * 12 + 4;
  return (
    <button
      type="button"
      className={cn(
        "flex min-h-6 w-full items-center gap-1 rounded-md px-1 py-0.5 text-left text-xs text-foreground/70 hover:bg-muted/60 cursor-pointer [@media(pointer:coarse)]:min-h-11",
        !touchMode && "-mx-1",
        touchMode && "min-h-11",
      )}
      style={{ paddingLeft: touchMode ? Math.min(indentPx, 24) : indentPx }}
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
