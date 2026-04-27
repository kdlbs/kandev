"use client";

import { useState } from "react";
import { IconCode, IconChevronDown, IconSend } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { formatRelativeTime } from "@/lib/utils";
import type { IssueComment } from "./types";

type TaskChatProps = {
  taskId: string;
  comments: IssueComment[];
  readOnly?: boolean;
};

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return `${minutes}m ${remaining}s`;
}

function AgentAvatar({ name }: { name: string }) {
  const initial = name.charAt(0).toUpperCase();
  return (
    <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center shrink-0">
      <span className="text-xs font-medium text-muted-foreground">{initial}</span>
    </div>
  );
}

function CommentEntry({ comment }: { comment: IssueComment }) {
  return (
    <div className="flex gap-3 py-3">
      <AgentAvatar name={comment.authorName} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-medium text-sm">{comment.authorName}</span>
          {comment.status && (
            <span className="text-xs text-muted-foreground">
              {comment.status}
              {comment.durationMs != null && ` after ${formatDuration(comment.durationMs)}`}
            </span>
          )}
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(comment.createdAt)}
          </span>
        </div>
        <div className="prose prose-sm mt-1 max-w-none text-sm whitespace-pre-wrap">
          {comment.content}
        </div>
        {comment.toolCalls && comment.toolCalls.length > 0 && (
          <Collapsible>
            <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground mt-1 cursor-pointer hover:text-foreground transition-colors">
              <IconCode className="h-3 w-3" />
              Worked -- ran {comment.toolCalls.length} commands
              <IconChevronDown className="h-3 w-3" />
            </CollapsibleTrigger>
            <CollapsibleContent>
              <div className="mt-2 space-y-1 text-xs font-mono bg-muted rounded-md p-2">
                {comment.toolCalls.map((tc) => (
                  <div key={tc.id} className="text-muted-foreground">
                    <span className="text-foreground">{tc.name}</span>
                    {tc.input && <span className="ml-1 opacity-70">{tc.input}</span>}
                  </div>
                ))}
              </div>
            </CollapsibleContent>
          </Collapsible>
        )}
      </div>
    </div>
  );
}

export function TaskChat({ taskId, comments, readOnly = false }: TaskChatProps) {
  const [input, setInput] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim()) return;
    // TODO: Wire to API once backend (Wave 3A) is ready
    void taskId;
    setInput("");
  };

  return (
    <div className="flex flex-col">
      {comments.length === 0 ? (
        <p className="text-sm text-muted-foreground py-4">No comments yet</p>
      ) : (
        <div className="divide-y divide-border/50">
          {comments.map((comment) => (
            <CommentEntry key={comment.id} comment={comment} />
          ))}
        </div>
      )}

      {!readOnly && (
        <form onSubmit={handleSubmit} className="flex gap-2 mt-4 pt-4 border-t border-border">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Add a comment..."
            className="flex-1 bg-muted rounded-md px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button type="submit" size="sm" variant="ghost" className="cursor-pointer shrink-0">
                <IconSend className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Send</TooltipContent>
          </Tooltip>
        </form>
      )}
    </div>
  );
}
