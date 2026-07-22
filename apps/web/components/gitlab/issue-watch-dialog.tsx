"use client";

import { GitLabWatchDialog } from "./watch-dialog";
import type {
  CreateIssueWatchRequest,
  UpdateIssueWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import type { IssueWatch } from "@/lib/types/gitlab";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: IssueWatch | null;
  workspaceId: string;
  onCreate: (request: CreateIssueWatchRequest) => Promise<unknown>;
  onUpdate: (id: string, request: UpdateIssueWatchRequest) => Promise<unknown>;
};

export function IssueWatchDialog(props: Props) {
  return (
    <GitLabWatchDialog
      kind="issue"
      {...props}
      onCreate={(request) => props.onCreate(request as CreateIssueWatchRequest)}
      onUpdate={(id, request) => props.onUpdate(id, request as UpdateIssueWatchRequest)}
    />
  );
}
