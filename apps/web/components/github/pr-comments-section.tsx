import { useMemo } from "react";
import type { PRComment } from "@/lib/types/github";
import {
  CollapsibleSection,
  AddToContextButton,
  AuthorAvatar,
  AuthorLink,
  formatTimeAgo,
} from "./pr-shared";

function buildCommentMessage(comment: PRComment, prUrl: string): string {
  const location = comment.path
    ? `\`${comment.path}${comment.line > 0 ? `:${comment.line}` : ""}\``
    : "";
  const header = location
    ? `Comment from **${comment.author}** on ${location}:`
    : `Comment from **${comment.author}**:`;
  return [header, comment.body, `PR: ${prUrl}`, "Please address this comment."].join("\n\n");
}

function buildThreadMessage(thread: CommentThread, prUrl: string): string {
  const parts = ["### Comment Thread", ""];
  for (const c of [thread.root, ...thread.replies]) {
    const loc = c.path ? ` on \`${c.path}${c.line > 0 ? `:${c.line}` : ""}\`` : "";
    parts.push(`**${c.author}**${loc}:`);
    parts.push(c.body);
    parts.push("");
  }
  parts.push(`PR: ${prUrl}`);
  parts.push("Please address this comment thread.");
  return parts.join("\n");
}

function buildAllCommentsMessage(comments: PRComment[], prUrl: string): string {
  const parts = ["### All PR Comments", ""];
  for (const c of comments) {
    const loc = c.path ? ` on \`${c.path}${c.line > 0 ? `:${c.line}` : ""}\`` : "";
    parts.push(`**${c.author}**${loc}:`);
    parts.push(c.body);
    parts.push("");
  }
  parts.push(`PR: ${prUrl}`);
  parts.push("Please address the comments above.");
  return parts.join("\n");
}

type CommentThread = {
  root: PRComment;
  replies: PRComment[];
};

function buildThreads(comments: PRComment[]): CommentThread[] {
  const byId = new Map<number, PRComment>();
  for (const c of comments) byId.set(c.id, c);

  const roots: PRComment[] = [];
  const repliesByParent = new Map<number, PRComment[]>();

  for (const c of comments) {
    if (c.in_reply_to === null || c.in_reply_to === 0 || !byId.has(c.in_reply_to)) {
      roots.push(c);
    } else {
      const existing = repliesByParent.get(c.in_reply_to) ?? [];
      existing.push(c);
      repliesByParent.set(c.in_reply_to, existing);
    }
  }

  return roots.map((root) => ({
    root,
    replies: repliesByParent.get(root.id) ?? [],
  }));
}

function CommentItem({
  comment,
  prUrl,
  onAddAsContext,
  isReply,
}: {
  comment: PRComment;
  prUrl: string;
  onAddAsContext: (message: string) => void;
  isReply?: boolean;
}) {
  return (
    <div className={isReply ? "ml-4 pl-2.5 border-l-2 border-border" : ""}>
      <div className="px-2.5 py-2 rounded-md border border-border bg-muted/30 space-y-1">
        <div className="flex items-center gap-2">
          <AuthorAvatar src={comment.author_avatar} author={comment.author} />
          <AuthorLink author={comment.author} />
          {comment.path && (
            <span className="text-[10px] text-muted-foreground font-mono truncate max-w-[180px]">
              {comment.path}
              {comment.line > 0 && `:${comment.line}`}
            </span>
          )}
          {!comment.path && !isReply && (
            <span className="text-[10px] text-muted-foreground">(general)</span>
          )}
          <span className="text-[10px] text-muted-foreground ml-auto shrink-0">
            {formatTimeAgo(comment.created_at)}
          </span>
          <AddToContextButton onClick={() => onAddAsContext(buildCommentMessage(comment, prUrl))} />
        </div>
        <p className="text-xs text-muted-foreground pl-7 line-clamp-4">{comment.body}</p>
      </div>
    </div>
  );
}

function ThreadBlock({
  thread,
  prUrl,
  onAddAsContext,
}: {
  thread: CommentThread;
  prUrl: string;
  onAddAsContext: (message: string) => void;
}) {
  const hasReplies = thread.replies.length > 0;
  return (
    <div className="space-y-1.5">
      <div className="flex items-start gap-1">
        <div className="flex-1">
          <CommentItem comment={thread.root} prUrl={prUrl} onAddAsContext={onAddAsContext} />
        </div>
        {hasReplies && (
          <AddToContextButton
            onClick={() => onAddAsContext(buildThreadMessage(thread, prUrl))}
            tooltip="Add whole thread to chat context"
          />
        )}
      </div>
      {thread.replies.map((reply) => (
        <CommentItem
          key={reply.id}
          comment={reply}
          prUrl={prUrl}
          onAddAsContext={onAddAsContext}
          isReply
        />
      ))}
    </div>
  );
}

export function CommentsSection({
  comments,
  prUrl,
  onAddAsContext,
}: {
  comments: PRComment[];
  prUrl: string;
  onAddAsContext: (message: string) => void;
}) {
  const threads = useMemo(() => buildThreads(comments), [comments]);

  return (
    <CollapsibleSection
      title="Comments"
      count={comments.length}
      defaultOpen
      onAddAll={() => onAddAsContext(buildAllCommentsMessage(comments, prUrl))}
      addAllLabel="Add all comments to chat context"
    >
      {comments.length === 0 && (
        <p className="text-xs text-muted-foreground px-2 py-2">No comments yet</p>
      )}
      {threads.map((thread) => (
        <ThreadBlock
          key={thread.root.id}
          thread={thread}
          prUrl={prUrl}
          onAddAsContext={onAddAsContext}
        />
      ))}
    </CollapsibleSection>
  );
}
