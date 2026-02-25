import { useMemo, useState } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import type { PRComment } from "@/lib/types/github";
import { CollapsibleSection, AddToContextButton, FeedbackItemRow } from "./pr-shared";

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

function CommentMetaBadge({ comment, isReply }: { comment: PRComment; isReply?: boolean }) {
  if (comment.path) {
    return (
      <span className="text-[10px] text-muted-foreground font-mono truncate max-w-[180px]">
        {comment.path}
        {comment.line > 0 && `:${comment.line}`}
      </span>
    );
  }
  if (!isReply) {
    return <span className="text-[10px] text-muted-foreground">(general)</span>;
  }
  return null;
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
    <FeedbackItemRow
      author={comment.author}
      authorAvatar={comment.author_avatar}
      body={comment.body}
      createdAt={comment.created_at}
      metaBadge={<CommentMetaBadge comment={comment} isReply={isReply} />}
      onAddAsContext={() => onAddAsContext(buildCommentMessage(comment, prUrl))}
      isReply={isReply}
    />
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
  const [showBotComments, setShowBotComments] = useState(false);
  const humanComments = useMemo(
    () => comments.filter((comment) => !comment.author_is_bot),
    [comments],
  );
  const botComments = useMemo(
    () => comments.filter((comment) => comment.author_is_bot),
    [comments],
  );
  const humanThreads = useMemo(() => buildThreads(humanComments), [humanComments]);
  const botThreads = useMemo(() => buildThreads(botComments), [botComments]);

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
      {humanThreads.map((thread) => (
        <ThreadBlock
          key={`human-${thread.root.id}`}
          thread={thread}
          prUrl={prUrl}
          onAddAsContext={onAddAsContext}
        />
      ))}
      {botComments.length > 0 && (
        <button
          type="button"
          onClick={() => setShowBotComments((current) => !current)}
          className="w-full px-2 py-1 text-left text-xs text-muted-foreground hover:text-foreground flex items-center gap-1.5 cursor-pointer"
        >
          {showBotComments ? (
            <IconChevronDown className="h-3.5 w-3.5" />
          ) : (
            <IconChevronRight className="h-3.5 w-3.5" />
          )}
          Bot comments ({botComments.length})
        </button>
      )}
      {showBotComments &&
        botThreads.map((thread) => (
          <ThreadBlock
            key={`bot-${thread.root.id}`}
            thread={thread}
            prUrl={prUrl}
            onAddAsContext={onAddAsContext}
          />
        ))}
    </CollapsibleSection>
  );
}
