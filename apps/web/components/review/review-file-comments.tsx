"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CommentForm } from "@/components/diff/comment-form";
import { MarkdownComment } from "@/components/task/simple/markdown-comment";
import { IconEdit, IconMessage, IconTrash } from "@tabler/icons-react";
import { useCommentsStore, type ReviewFileComment } from "@/lib/state/slices/comments";
import { buildReviewFileComment } from "@/lib/state/slices/comments/review-file";
import type { ReviewFile } from "./types";

function useFileComments(file: ReviewFile, sessionId: string) {
  const byId = useCommentsStore((state) => state.byId);
  const ids = useCommentsStore((state) => state.bySession[sessionId]);
  const hydrate = useCommentsStore((state) => state.hydrateSession);
  useEffect(() => {
    hydrate(sessionId);
  }, [hydrate, sessionId]);
  return useMemo(
    () =>
      (ids ?? []).flatMap((id) => {
        const c = byId[id];
        return c?.source === "review-file" &&
          c.filePath === file.path &&
          c.repositoryName === (file.repository_name ?? "")
          ? [c]
          : [];
      }),
    [byId, ids, file.path, file.repository_name],
  );
}

function FileCommentCard({ comment }: { comment: ReviewFileComment }) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const editRef = useRef<HTMLButtonElement>(null);
  const update = useCommentsStore((state) => state.updateComment);
  const remove = useCommentsStore((state) => state.removeComment);
  const close = () => {
    setEditing(false);
    requestAnimationFrame(() => editRef.current?.focus());
  };
  return (
    <div
      className="group min-w-0 rounded-md border border-border bg-card p-2 font-mono shadow-sm"
      data-testid="review-file-comment-card"
      onKeyDown={(event) => {
        if (editing && event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          close();
        }
      }}
    >
      {editing ? (
        <CommentForm
          initialContent={comment.text}
          isEditing
          onCancel={close}
          onSubmit={(text) => {
            update(comment.id, { text });
            close();
          }}
        />
      ) : (
        <>
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <div className="flex min-w-0 items-center gap-1.5 text-xs">
              <IconMessage className="h-3.5 w-3.5 shrink-0 text-blue-500" />
              <span className="font-medium">{t("review:fileComment")}</span>
            </div>
            <div className="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 max-md:opacity-100 [@media(pointer:coarse)]:opacity-100">
              <Button
                ref={editRef}
                size="icon"
                variant="ghost"
                aria-label={t("diff:editComment")}
                title={t("diff:editComment")}
                className="size-7 cursor-pointer p-0 max-md:size-11 [@media(pointer:coarse)]:size-11"
                onClick={() => setEditing(true)}
              >
                <IconEdit className="h-3 w-3" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("diff:deleteComment")}
                title={t("diff:deleteComment")}
                className="size-7 cursor-pointer p-0 hover:text-destructive max-md:size-11 [@media(pointer:coarse)]:size-11"
                onClick={() => remove(comment.id)}
              >
                <IconTrash className="h-3 w-3" />
              </Button>
            </div>
          </div>
          <div className="[&>.markdown-body]:font-mono! [&>.markdown-body]:text-xs! [&>.markdown-body]:leading-relaxed [&_p]:my-0">
            <MarkdownComment content={comment.text} />
          </div>
        </>
      )}
    </div>
  );
}

export function ReviewFileComments({
  file,
  sessionId,
  creating,
  onClose,
}: {
  file: ReviewFile;
  sessionId: string;
  creating: boolean;
  onClose: () => void;
}) {
  const comments = useFileComments(file, sessionId);
  const regionRef = useRef<HTMLDivElement>(null);
  const add = useCommentsStore((state) => state.addComment);
  const close = () => {
    const opener = regionRef.current?.parentElement?.querySelector<HTMLElement>(
      "[data-review-comment-opener]",
    );
    onClose();
    requestAnimationFrame(() => opener?.focus());
  };
  if (!creating && comments.length === 0) return null;
  return (
    <div
      ref={regionRef}
      data-testid="review-file-comments"
      className="min-w-0 space-y-2 border-b px-3 py-1.5 [overflow-wrap:anywhere] [&_button]:h-7 max-md:[&_button]:min-h-11 [@media(pointer:coarse)]:[&_button]:min-h-11"
      onKeyDown={(event) => {
        if (creating && event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          close();
        }
      }}
    >
      <p className={creating ? "break-all text-xs text-muted-foreground" : "sr-only"}>
        {[file.repository_name, file.path].filter(Boolean).join("/")}
      </p>
      {comments.map((comment) => (
        <FileCommentCard key={comment.id} comment={comment} />
      ))}
      {creating && (
        <CommentForm
          onCancel={close}
          onSubmit={(text) => {
            const comment = buildReviewFileComment(
              {
                sessionId,
                filePath: file.path,
                repositoryName: file.repository_name ?? "",
                repositoryId: file.repository_id,
              },
              text,
            );
            if (comment) {
              add(comment);
              close();
            }
          }}
        />
      )}
    </div>
  );
}
