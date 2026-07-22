"use client";

import { GitLabWatchDialog } from "./watch-dialog";
import type {
  CreateReviewWatchRequest,
  UpdateReviewWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import type { ReviewWatch } from "@/lib/types/gitlab";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: ReviewWatch | null;
  workspaceId: string;
  onCreate: (request: CreateReviewWatchRequest) => Promise<unknown>;
  onUpdate: (id: string, request: UpdateReviewWatchRequest) => Promise<unknown>;
};

export function ReviewWatchDialog(props: Props) {
  return (
    <GitLabWatchDialog
      kind="review"
      {...props}
      onCreate={(request) => props.onCreate(request as CreateReviewWatchRequest)}
      onUpdate={(id, request) => props.onUpdate(id, request as UpdateReviewWatchRequest)}
    />
  );
}
