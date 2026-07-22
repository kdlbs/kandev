"use client";

import { GitLabWatchTable, type WatchTableProps } from "./watch-table";
import type { ReviewWatch } from "@/lib/types/gitlab";

export function ReviewWatchTable(props: WatchTableProps<ReviewWatch>) {
  return <GitLabWatchTable {...props} />;
}
