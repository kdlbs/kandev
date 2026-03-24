"use client";

import { memo } from "react";
import ReactMarkdown from "react-markdown";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconCode } from "@tabler/icons-react";
import { remarkPlugins, markdownComponents } from "@/components/shared/markdown-components";
import { toRelativePath } from "@/lib/utils";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";

interface MarkdownPreviewToolbarProps {
  path: string;
  worktreePath?: string;
  onTogglePreview: () => void;
}

function MarkdownPreviewToolbar({
  path,
  worktreePath,
  onTogglePreview,
}: MarkdownPreviewToolbarProps) {
  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
          <span className="text-xs text-muted-foreground/60">Preview</span>
        </div>
      }
      right={
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                onClick={onTogglePreview}
                className="h-8 w-8 p-0 cursor-pointer text-foreground"
                data-testid="markdown-preview-toggle"
              >
                <IconCode className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Show code</TooltipContent>
          </Tooltip>
        </div>
      }
    />
  );
}

interface MarkdownPreviewContentProps {
  path: string;
  content: string;
  worktreePath?: string;
  onTogglePreview: () => void;
}

export const MarkdownPreviewContent = memo(function MarkdownPreviewContent({
  path,
  content,
  worktreePath,
  onTogglePreview,
}: MarkdownPreviewContentProps) {
  return (
    <div className="flex h-full flex-col" data-testid="markdown-preview">
      <MarkdownPreviewToolbar
        path={path}
        worktreePath={worktreePath}
        onTogglePreview={onTogglePreview}
      />
      <div className="flex-1 overflow-auto p-6">
        <div className="markdown-body max-w-3xl">
          <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
            {content}
          </ReactMarkdown>
        </div>
      </div>
    </div>
  );
});
