"use client";

import { GitLabWatchTable, type WatchTableProps } from "./watch-table";
import type { IssueWatch } from "@/lib/types/gitlab";

export function IssueWatchTable(props: WatchTableProps<IssueWatch>) {
  return <GitLabWatchTable {...props} />;
}
