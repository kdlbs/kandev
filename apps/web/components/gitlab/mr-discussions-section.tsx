"use client";

import { useState } from "react";
import { IconCheck, IconMessagePlus, IconSend } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type { GitLabMRDiscussion } from "@/lib/types/gitlab";
import { CollapsibleSection, formatTimeAgo, PRMarkdownBody } from "@/components/github/pr-shared";

function discussionLocation(discussion: GitLabMRDiscussion): string {
  if (!discussion.path) return "";
  const line = discussion.line || discussion.old_line;
  return `\`${discussion.path}${line ? `:${line}` : ""}\``;
}

export function buildDiscussionContext(discussion: GitLabMRDiscussion, mrUrl: string): string {
  const parts = ["### Merge request discussion", ""];
  const location = discussionLocation(discussion);
  if (location) parts.push(`Location: ${location}`, "");
  for (const note of discussion.notes) {
    parts.push(`**${note.author}**:`, note.body, "");
  }
  parts.push(`Merge request: ${mrUrl}`, "Please address this discussion.");
  return parts.join("\n");
}

export function buildAllDiscussionsContext(
  discussions: GitLabMRDiscussion[],
  mrUrl: string,
): string {
  return discussions
    .map((discussion) => buildDiscussionContext(discussion, mrUrl))
    .join("\n\n---\n\n");
}

type DiscussionProps = {
  discussion: GitLabMRDiscussion;
  busy: boolean;
  onReply: (discussionId: string, body: string) => Promise<boolean>;
  onResolve: (discussionId: string) => Promise<boolean>;
  onAddContext: (content: string) => void;
  mrUrl: string;
};

function Discussion({
  discussion,
  busy,
  onReply,
  onResolve,
  onAddContext,
  mrUrl,
}: DiscussionProps) {
  const [reply, setReply] = useState("");
  const submitReply = async () => {
    const body = reply.trim();
    if (!body) return;
    if (await onReply(discussion.id, body)) setReply("");
  };
  const location = discussionLocation(discussion);

  return (
    <article
      className="rounded-md border border-border bg-muted/20 p-2.5"
      data-testid={`gitlab-discussion-${discussion.id}`}
    >
      <header className="mb-2 flex min-w-0 items-center gap-2">
        {location && (
          <span className="min-w-0 truncate font-mono text-[11px]">
            {location.replaceAll("`", "")}
          </span>
        )}
        {discussion.resolved && (
          <Badge variant="outline" className="ml-auto text-[10px]">
            Resolved
          </Badge>
        )}
        <Button
          size="icon-sm"
          variant="ghost"
          className="ml-auto h-9 w-9 shrink-0 cursor-pointer sm:h-7 sm:w-7"
          aria-label="Add discussion to task context"
          onClick={() => onAddContext(buildDiscussionContext(discussion, mrUrl))}
        >
          <IconMessagePlus className="h-3.5 w-3.5" />
        </Button>
      </header>
      <div className="space-y-2">
        {discussion.notes.map((note) => (
          <div key={note.id} className="border-l-2 border-border pl-2.5">
            <div className="flex items-center gap-2 text-xs">
              <strong>{note.author}</strong>
              <span className="text-[10px] text-muted-foreground">
                {formatTimeAgo(note.created_at)}
              </span>
            </div>
            <PRMarkdownBody body={note.body} />
          </div>
        ))}
      </div>
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-end">
        <Textarea
          value={reply}
          onChange={(event) => setReply(event.target.value)}
          placeholder="Reply to this discussion"
          aria-label="Discussion reply"
          className="min-h-20 flex-1 resize-y text-sm"
        />
        <div className="flex gap-2">
          {discussion.resolvable && !discussion.resolved && (
            <Button
              size="sm"
              variant="outline"
              className="h-11 flex-1 cursor-pointer gap-1 sm:h-9"
              disabled={busy}
              onClick={() => void onResolve(discussion.id)}
            >
              <IconCheck className="h-3.5 w-3.5" /> Resolve
            </Button>
          )}
          <Button
            size="sm"
            className="h-11 flex-1 cursor-pointer gap-1 sm:h-9"
            disabled={busy || !reply.trim()}
            onClick={() => void submitReply()}
          >
            <IconSend className="h-3.5 w-3.5" /> Reply
          </Button>
        </div>
      </div>
    </article>
  );
}

export function MRDiscussionsSection({
  discussions,
  mrUrl,
  busy,
  onReply,
  onResolve,
  onAddContext,
}: {
  discussions: GitLabMRDiscussion[];
  mrUrl: string;
  busy: boolean;
  onReply: DiscussionProps["onReply"];
  onResolve: DiscussionProps["onResolve"];
  onAddContext: DiscussionProps["onAddContext"];
}) {
  return (
    <CollapsibleSection
      title="Discussions"
      count={discussions.length}
      defaultOpen
      onAddAll={
        discussions.length
          ? () => onAddContext(buildAllDiscussionsContext(discussions, mrUrl))
          : undefined
      }
      addAllLabel="Add all discussions to task context"
    >
      {discussions.length === 0 && (
        <p className="px-2 py-2 text-xs text-muted-foreground">No discussions yet</p>
      )}
      {discussions.map((discussion) => (
        <Discussion
          key={discussion.id}
          discussion={discussion}
          busy={busy}
          onReply={onReply}
          onResolve={onResolve}
          onAddContext={onAddContext}
          mrUrl={mrUrl}
        />
      ))}
    </CollapsibleSection>
  );
}
