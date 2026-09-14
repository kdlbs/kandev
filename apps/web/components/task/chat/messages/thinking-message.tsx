"use client";

import { useState, memo } from "react";
import { IconBrain } from "@tabler/icons-react";
import type { Message } from "@/lib/types/http";
import type { RichMetadata } from "@/components/task/chat/types";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { ExpandableRow } from "./expandable-row";
import { useTranslation } from "react-i18next";

// Strip markdown formatting for inline display
function stripMarkdown(text: string): string {
  return (
    text
      // Bold/italic: **text** or __text__ or *text* or _text_
      .replace(/(^|[^\w])(\*\*|__)(?=\S)(.*?\S)\2(?=$|[^\w])/g, "$1$3")
      .replace(/(^|[^\w])(\*|_)(?=\S)(.*?\S)\2(?=$|[^\w])/g, "$1$3")
      // Code: `code` or ```code```
      .replace(/`{1,3}([^`]+)`{1,3}/g, "$1")
      // Links: [text](url) -> text
      .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
      // Headers: ## text -> text
      .replace(/^#{1,6}\s+/gm, "")
      // Strikethrough: ~~text~~
      .replace(/~~(.*?)~~/g, "$1")
      // Markdown-only block markers do not provide preview text
      .replace(/^[ \t]*#{1,6}[ \t]*$/gm, "")
      .replace(/^[ \t]*(?:\*{1,3}|_{1,3}|~{2,}|`{1,3})[ \t]*$/gm, "")
      .replace(/^[ \t]*(?:`{3,}|~{3,})[ \t]*[\w-]*[ \t]*$/gm, "")
      .replace(/^[ \t]*(?:(?:\*|_|-)[ \t]*){3,}$/gm, "")
      .replace(/^[ \t]*(?:[-+*]|\d+[.)]|>)[ \t]*$/gm, "")
      // Remove any remaining special chars at start/end
      .trim()
  );
}

function firstMeaningfulLine(text: string): string {
  for (const line of text.split(/\r?\n/)) {
    const plainText = stripMarkdown(line);
    if (plainText) return plainText;
  }

  return "";
}

export const ThinkingMessage = memo(function ThinkingMessage({
  comment,
  worktreePath,
  onOpenFile,
}: {
  comment: Message;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as RichMetadata | undefined;
  const text = metadata?.thinking ?? comment.content;

  if (!text) return null;

  // Check if the message is short enough to display inline
  // Short = no newlines and less than 100 characters
  const isShort = !text.includes("\n") && text.length <= 100;
  const displayText = isShort ? stripMarkdown(text) : text;
  const preview = isShort ? "" : firstMeaningfulLine(text);

  return (
    <ExpandableRow
      icon={<IconBrain className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex min-w-0 items-center gap-2 text-xs">
          <span className="inline-flex min-w-0 items-center gap-1.5">
            <span className="shrink-0 font-mono text-xs text-muted-foreground">
              {t("task:thinking")}
            </span>
            {isShort && (
              <span className="min-w-0 break-words whitespace-normal text-xs text-muted-foreground/80">
                {displayText}
              </span>
            )}
          </span>
          {!isShort && preview && (
            <span
              data-testid="thinking-message-preview"
              className="min-w-0 flex-1 truncate text-xs text-muted-foreground/80"
            >
              {preview}
            </span>
          )}
        </div>
      }
      hasExpandableContent={!isShort && !!text}
      isExpanded={isExpanded}
      onToggle={() => setIsExpanded(!isExpanded)}
    >
      {!isShort && (
        <div className="pl-4 border-l-2 border-border/30">
          <div className="markdown-body max-w-none text-xs text-foreground/70 [&>*]:my-1 [&>p]:my-1 [&>ul]:my-1 [&>ol]:my-1 [&_strong]:text-foreground/80">
            <MemoizedMarkdown content={text} worktreePath={worktreePath} onOpenFile={onOpenFile} />
          </div>
        </div>
      )}
    </ExpandableRow>
  );
});
