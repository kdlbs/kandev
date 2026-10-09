"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { memo } from "react";
import type { CommentContextItem } from "@/lib/types/context";
import { getFileName } from "@/lib/utils/file-path";
import { ContextChip } from "./context-chip";
import { CommentDisplay } from "@/components/diff/comment-display";

export const CommentItem = memo(function CommentItem({ item }: { item: CommentContextItem }) {
  const { t } = useTranslation();
  const fileName = getFileName(item.filePath);

  const preview = (
    <div className="space-y-1.5">
      <div className="text-xs font-medium text-muted-foreground truncate" title={item.filePath}>
        {fileName}
      </div>
      <div className="space-y-1">
        {item.comments.map((comment) =>
          comment.source === "review-file" ? (
            <div key={comment.id} className="space-y-1 text-xs [overflow-wrap:anywhere]">
              <p className="text-muted-foreground">{t("review:fileComment")}</p>
              <p>{[comment.repositoryName, comment.filePath].filter(Boolean).join("/")}</p>
              <p className="whitespace-pre-wrap">{comment.text}</p>
              <Button
                variant="ghost"
                className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                onClick={() => item.onRemoveComment(comment.id)}
              >
                {t("common:delete")}
              </Button>
            </div>
          ) : (
            <CommentDisplay
              key={comment.id}
              comment={comment}
              compact
              onDelete={() => item.onRemoveComment(comment.id)}
            />
          ),
        )}
      </div>
    </div>
  );

  return (
    <ContextChip
      kind="comment"
      label={item.label}
      preview={preview}
      onClick={item.onOpen}
      onRemove={item.onRemove}
    />
  );
});
