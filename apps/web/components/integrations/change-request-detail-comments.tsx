import { useMemo, useState } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { AddToContextButton, CollapsibleSection } from "@/components/github/pr-shared";
import { t } from "@/lib/i18n";
import { ChangeRequestActionButton } from "./change-request-detail-header";
import { ChangeRequestFeedbackRow } from "./change-request-detail-feedback";
import type {
  ChangeRequestDetailAction,
  ChangeRequestDetailComment,
  ChangeRequestDetailModel,
  ChangeRequestDetailProps,
} from "./change-request-detail";

type CommentThread = { root: ChangeRequestDetailComment; replies: ChangeRequestDetailComment[] };

function commentThreads(comments: ChangeRequestDetailComment[]): CommentThread[] {
  const byId = new Set(comments.map((comment) => comment.id));
  const replies = new Map<string, ChangeRequestDetailComment[]>();
  for (const comment of comments) {
    if (!comment.parentId || !byId.has(comment.parentId)) continue;
    replies.set(comment.parentId, [...(replies.get(comment.parentId) ?? []), comment]);
  }
  return comments
    .filter((comment) => !comment.parentId || !byId.has(comment.parentId))
    .map((root) => ({ root, replies: replies.get(root.id) ?? [] }));
}

function commentLocation(comment: ChangeRequestDetailComment) {
  return comment.path ? ` on \`${comment.path}${comment.line ? `:${comment.line}` : ""}\`` : "";
}

function buildCommentContext(comment: ChangeRequestDetailComment, url: string) {
  return [
    `Comment from **${comment.author.name}**${commentLocation(comment)}:`,
    comment.body,
    `PR: ${url}`,
    "Please address this comment.",
  ].join("\n\n");
}

function buildThreadContext(thread: CommentThread, url: string) {
  const lines = ["### Comment Thread", ""];
  for (const comment of [thread.root, ...thread.replies]) {
    lines.push(`**${comment.author.name}**${commentLocation(comment)}:`, comment.body, "");
  }
  lines.push(`PR: ${url}`, "Please address this comment thread.");
  return lines.join("\n");
}

function buildAllCommentsContext(comments: ChangeRequestDetailComment[], url: string) {
  const lines = ["### All PR Comments", ""];
  for (const comment of comments) {
    lines.push(`**${comment.author.name}**${commentLocation(comment)}:`, comment.body, "");
  }
  lines.push(`PR: ${url}`, "Please address the comments above.");
  return lines.join("\n");
}

function TextAction({
  action,
  threadId,
  busy,
  onAction,
}: {
  action: ChangeRequestDetailAction;
  threadId?: string;
  busy: boolean;
  onAction?: ChangeRequestDetailProps["onAction"];
}) {
  const [body, setBody] = useState("");
  const label = threadId
    ? t("integrations:replyToThread", { threadId })
    : t("integrations:addReviewComment");
  return (
    <form
      className="space-y-2 rounded-md border border-border p-2"
      onSubmit={(event) => {
        event.preventDefault();
        const trimmed = body.trim();
        if (!trimmed) return;
        void onAction?.({ actionId: action.id, body: trimmed, ...(threadId ? { threadId } : {}) });
        setBody("");
      }}
    >
      <Label htmlFor={`${action.id}-${threadId ?? "review"}`}>{label}</Label>
      <Textarea
        id={`${action.id}-${threadId ?? "review"}`}
        value={body}
        onChange={(event) => setBody(event.target.value)}
        className="min-h-11"
      />
      <ChangeRequestActionButton
        action={action}
        busy={busy}
        onAction={() => {}}
        ariaLabel={threadId ? label : undefined}
        type="submit"
      />
    </form>
  );
}

function commentMetadata(comment: ChangeRequestDetailComment, reply = false) {
  if (comment.path) {
    return (
      <span className="max-w-[180px] truncate font-mono text-[10px] text-muted-foreground">
        {comment.path}
        {comment.line ? `:${comment.line}` : ""}
      </span>
    );
  }
  return reply ? null : (
    <span className="text-[10px] text-muted-foreground">{t("integrations:general")}</span>
  );
}

function CommentThreadBlock({
  thread,
  props,
  detail,
  replyAction,
}: {
  thread: CommentThread;
  props: ChangeRequestDetailProps;
  detail: ChangeRequestDetailModel;
  replyAction?: ChangeRequestDetailAction;
}) {
  return (
    <div className="space-y-1.5" data-testid={`change-request-comment-thread-${thread.root.id}`}>
      <div className="flex items-start gap-1">
        <div className="flex-1">
          <ChangeRequestFeedbackRow
            person={thread.root.author}
            body={thread.root.body}
            createdAt={thread.root.createdAt}
            metadata={commentMetadata(thread.root)}
            copyUrl={thread.root.url}
            copyTestId={`change-request-comment-copy-${thread.root.id}`}
            onAdd={
              props.onAddContext
                ? () =>
                    props.onAddContext?.("comment", buildCommentContext(thread.root, detail.url))
                : undefined
            }
          />
        </div>
        {thread.replies.length > 0 && props.onAddContext ? (
          <div data-testid={`change-request-thread-context-${thread.root.id}`}>
            <AddToContextButton
              onClick={() =>
                props.onAddContext?.("comment", buildThreadContext(thread, detail.url))
              }
              tooltip={t("integrations:addWholeThreadToChatContext")}
            />
          </div>
        ) : null}
      </div>
      {thread.replies.map((reply) => (
        <ChangeRequestFeedbackRow
          key={reply.id}
          person={reply.author}
          body={reply.body}
          createdAt={reply.createdAt}
          metadata={commentMetadata(reply, true)}
          copyUrl={reply.url}
          copyTestId={`change-request-comment-copy-${reply.id}`}
          onAdd={
            props.onAddContext
              ? () => props.onAddContext?.("comment", buildCommentContext(reply, detail.url))
              : undefined
          }
          reply
        />
      ))}
      {replyAction ? (
        <TextAction
          action={replyAction}
          threadId={thread.root.id}
          busy={props.busyActionId === replyAction.id}
          onAction={props.onAction}
        />
      ) : null}
    </div>
  );
}

export function ChangeRequestComments({
  props,
  detail,
}: {
  props: ChangeRequestDetailProps;
  detail: ChangeRequestDetailModel;
}) {
  const [showBotComments, setShowBotComments] = useState(false);
  const humanComments = useMemo(
    () => detail.comments.filter((comment) => !comment.author.isBot),
    [detail.comments],
  );
  const botComments = useMemo(
    () => detail.comments.filter((comment) => comment.author.isBot),
    [detail.comments],
  );
  const threads = useMemo(() => commentThreads(humanComments), [humanComments]);
  const botThreads = useMemo(() => commentThreads(botComments), [botComments]);
  const replyAction = props.actions?.find((action) => action.placement === "thread");
  const commentAction = props.actions?.find((action) => action.placement === "comment");
  return (
    <CollapsibleSection
      title={t("integrations:comments")}
      count={detail.comments.length}
      defaultOpen
      onAddAll={
        props.onAddContext
          ? () =>
              props.onAddContext?.("comment", buildAllCommentsContext(detail.comments, detail.url))
          : undefined
      }
      addAllLabel={t("integrations:addAllCommentsToChatContext")}
    >
      {detail.comments.length === 0 ? (
        <p className="px-2 py-2 text-xs text-muted-foreground">{t("integrations:noCommentsYet")}</p>
      ) : null}
      {threads.map((thread) => (
        <CommentThreadBlock
          key={thread.root.id}
          thread={thread}
          props={props}
          detail={detail}
          replyAction={replyAction}
        />
      ))}
      {botComments.length > 0 ? (
        <button
          type="button"
          className="min-h-11 w-full cursor-pointer px-2 py-1 text-left text-xs text-muted-foreground hover:text-foreground sm:min-h-0"
          onClick={() => setShowBotComments((current) => !current)}
        >
          {showBotComments ? (
            <IconChevronDown className="mr-1.5 inline h-3.5 w-3.5" />
          ) : (
            <IconChevronRight className="mr-1.5 inline h-3.5 w-3.5" />
          )}
          {t("integrations:botComments", { count: botComments.length })}
        </button>
      ) : null}
      {showBotComments
        ? botThreads.map((thread) => (
            <CommentThreadBlock
              key={thread.root.id}
              thread={thread}
              props={props}
              detail={detail}
              replyAction={replyAction}
            />
          ))
        : null}
      {commentAction ? (
        <TextAction
          action={commentAction}
          busy={props.busyActionId === commentAction.id}
          onAction={props.onAction}
        />
      ) : null}
    </CollapsibleSection>
  );
}
