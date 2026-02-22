"use client";

import type { ContextItem } from "@/lib/types/context";
import { PlanItem } from "./plan-item";
import { FileItem } from "./file-item";
import { PromptItem } from "./prompt-item";
import { CommentItem } from "./comment-item";
import { PlanCommentItem } from "./plan-comment-item";
import { ImageItem } from "./image-item";
import { PRFeedbackItem } from "./pr-feedback-item";

export function ContextItemRenderer({
  item,
  sessionId,
}: {
  item: ContextItem;
  sessionId?: string | null;
}) {
  switch (item.kind) {
    case "plan":
      return <PlanItem item={item} />;
    case "file":
      return <FileItem item={item} sessionId={sessionId} />;
    case "prompt":
      return <PromptItem item={item} />;
    case "comment":
      return <CommentItem item={item} />;
    case "plan-comment":
      return <PlanCommentItem item={item} />;
    case "image":
      return <ImageItem item={item} />;
    case "pr-feedback":
      return <PRFeedbackItem item={item} />;
  }
}
