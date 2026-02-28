"use client";

import { useState, memo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { IconBrain } from "@tabler/icons-react";
import type { Message } from "@/lib/types/http";
import type { RichMetadata } from "@/components/task/chat/types";
import { ExpandableRow } from "./expandable-row";

// Strip markdown formatting for inline display
function stripMarkdown(text: string): string {
  return (
    text
      // Bold/italic: **text** or __text__ or *text* or _text_
      .replace(/(\*\*|__)(.*?)\1/g, "$2")
      .replace(/(\*|_)(.*?)\1/g, "$2")
      // Code: `code` or ```code```
      .replace(/`{1,3}([^`]+)`{1,3}/g, "$1")
      // Links: [text](url) -> text
      .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
      // Headers: ## text -> text
      .replace(/^#{1,6}\s+/gm, "")
      // Strikethrough: ~~text~~
      .replace(/~~(.*?)~~/g, "$1")
      // Remove any remaining special chars at start/end
      .trim()
  );
}

export const ThinkingMessage = memo(function ThinkingMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as RichMetadata | undefined;
  const text = metadata?.thinking ?? comment.content;

  if (!text) return null;

  // Check if the message is short enough to display inline
  // Short = no newlines and less than 100 characters
  const isShort = !text.includes("\n") && text.length <= 100;
  const displayText = isShort ? stripMarkdown(text) : text;

  return (
    <ExpandableRow
      icon={<IconBrain className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-xs text-muted-foreground">Thinking</span>
            {isShort && <span className="text-xs text-muted-foreground/80">{displayText}</span>}
          </span>
        </div>
      }
      hasExpandableContent={!isShort && !!text}
      isExpanded={isExpanded}
      onToggle={() => setIsExpanded(!isExpanded)}
    >
      {!isShort && (
        <div className="pl-4 border-l-2 border-border/30">
          <div className="markdown-body max-w-none text-xs text-foreground/70 [&>*]:my-1 [&>p]:my-1 [&>ul]:my-1 [&>ol]:my-1 [&_strong]:text-foreground/80">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{text}</ReactMarkdown>
          </div>
        </div>
      )}
    </ExpandableRow>
  );
});
