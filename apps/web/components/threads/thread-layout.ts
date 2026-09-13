import type { ThreadLayout } from "@/lib/state/slices/ui/thread-view-types";

export type ThreadLayoutResult = {
  layout: ThreadLayout;
  rows: 1 | 2;
  columns: number;
  heightFallback: boolean;
};

export function resolveThreadLayout({
  layout,
  isMobile,
  taskCount,
  contentHeight,
}: {
  layout: ThreadLayout;
  isMobile: boolean;
  taskCount: number;
  contentHeight: number | null;
}): ThreadLayoutResult {
  const columns: ThreadLayoutResult = {
    layout: "columns",
    rows: 1,
    columns: taskCount,
    heightFallback: false,
  };
  if (layout !== "grid" || isMobile || taskCount === 0) return columns;
  if (contentHeight === null) return columns;
  if (taskCount > 1 && contentHeight < 612) return { ...columns, heightFallback: true };
  return {
    layout: "grid",
    rows: taskCount === 1 ? 1 : 2,
    columns: Math.ceil(taskCount / 2),
    heightFallback: false,
  };
}
