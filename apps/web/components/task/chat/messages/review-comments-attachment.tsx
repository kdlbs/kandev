"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { IconChevronDown, IconChevronRight, IconMessage } from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";
import type { DiffComment } from "@/lib/diff/types";
import { formatLineRange } from "@/lib/diff";

interface ReviewCommentsAttachmentProps {
  /** Review comments from the message */
  comments: DiffComment[];
  /** Additional class name */
  className?: string;
}

/**
 * Renders review comments attached to a sent user message.
 * Shows a compact summary that expands to show full comment details.
 */
export function ReviewCommentsAttachment({ comments, className }: ReviewCommentsAttachmentProps) {
  const [isOpen, setIsOpen] = useState(false);

  if (!comments || comments.length === 0) {
    return null;
  }

  // Group comments by file
  const byFile: Record<string, DiffComment[]> = {};
  for (const comment of comments) {
    if (!byFile[comment.filePath]) {
      byFile[comment.filePath] = [];
    }
    byFile[comment.filePath].push(comment);
  }

  const fileCount = Object.keys(byFile).length;
  const totalComments = comments.length;

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <div className={cn("mt-2 rounded-lg border border-border bg-muted/30", className)}>
        {/* Header */}
        <CollapsibleTrigger asChild>
          <Button
            variant="ghost"
            className="flex w-full items-center justify-start gap-2 px-3 py-2"
          >
            {isOpen ? (
              <IconChevronDown className="h-4 w-4 shrink-0" />
            ) : (
              <IconChevronRight className="h-4 w-4 shrink-0" />
            )}

            <IconMessage className="h-4 w-4 shrink-0 text-blue-500" />

            <span className="text-sm font-medium">Review Comments</span>

            <Badge variant="secondary" className="ml-auto text-xs">
              {totalComments} comment{totalComments !== 1 ? "s" : ""} on {fileCount} file
              {fileCount !== 1 ? "s" : ""}
            </Badge>
          </Button>
        </CollapsibleTrigger>

        {/* Expanded content */}
        <CollapsibleContent>
          <div className="border-t border-border/50 px-3 py-2">
            {Object.entries(byFile).map(([filePath, fileComments]) => (
              <div key={filePath} className="mb-3 last:mb-0">
                {/* File header */}
                <div className="mb-1.5 flex items-center gap-1.5 text-xs">
                  <span className="font-medium text-muted-foreground">
                    {filePath.split("/").pop()}
                  </span>
                  <span className="text-muted-foreground/60">({fileComments.length})</span>
                </div>

                {/* Comments */}
                <div className="space-y-2">
                  {fileComments.map((comment) => (
                    <div
                      key={comment.id}
                      className="rounded-md border border-border/50 bg-card p-2"
                    >
                      {/* Line info */}
                      <div className="mb-1 flex items-center gap-1.5 text-[10px] text-muted-foreground">
                        <span className="font-medium">
                          {formatLineRange(comment.startLine, comment.endLine)}
                        </span>
                        <span>({comment.side === "additions" ? "new" : "old"})</span>
                      </div>

                      {/* Code preview */}
                      {comment.codeContent && (
                        <pre className="mb-1.5 overflow-x-auto rounded bg-muted p-1.5 text-[10px] leading-tight">
                          <code>{comment.codeContent}</code>
                        </pre>
                      )}

                      {/* Comment text */}
                      <p className="whitespace-pre-wrap text-xs leading-relaxed">{comment.text}</p>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  );
}

// Re-export from unified comment system
export { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
