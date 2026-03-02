"use client";

import { useState, memo } from "react";
import ReactMarkdown from "react-markdown";
import { IconMinus, IconPlus, IconMaximize } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { remarkPlugins, markdownComponents } from "@/components/shared/markdown-components";
import type { Message } from "@/lib/types/http";

function parsePlanContent(text: string): { title: string; body: string } {
  const lines = text.split("\n");
  const firstContentIdx = lines.findIndex((l) => l.trim().length > 0);
  if (firstContentIdx === -1) return { title: "Agent Plan", body: "" };
  const firstLine = lines[firstContentIdx];
  const isHeading = /^#{1,6}\s+/.test(firstLine);
  const title = isHeading ? firstLine.replace(/^#{1,6}\s+/, "").trim() : firstLine.trim();
  const body = isHeading ? lines.slice(firstContentIdx + 1).join("\n") : text;
  return { title: title || "Agent Plan", body };
}

function PlanMarkdownBody({ text, className }: { text: string; className?: string }) {
  return (
    <div
      className={`markdown-body max-w-none text-sm [&>*]:my-2 [&>p]:my-2 [&>ul]:my-2 [&>ol]:my-2 ${className ?? ""}`}
    >
      <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {text}
      </ReactMarkdown>
    </div>
  );
}

export const AgentPlanMessage = memo(function AgentPlanMessage({ comment }: { comment: Message }) {
  const [collapsed, setCollapsed] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const text = comment.content;

  if (!text) return null;

  const { title, body } = parsePlanContent(text);

  return (
    <>
      <div className="rounded-lg border border-border/60 overflow-hidden">
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-border/40">
          <span className="text-sm font-medium">{title}</span>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              aria-label={collapsed ? "Expand plan" : "Collapse plan"}
              className="h-7 w-7 cursor-pointer"
              onClick={() => setCollapsed(!collapsed)}
            >
              {collapsed ? <IconPlus className="h-4 w-4" /> : <IconMinus className="h-4 w-4" />}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Open plan details"
              className="h-7 w-7 cursor-pointer"
              onClick={() => setDialogOpen(true)}
            >
              <IconMaximize className="h-4 w-4" />
            </Button>
          </div>
        </div>
        {!collapsed && (
          <div className="max-h-[300px] overflow-y-auto">
            <div className="px-5 py-4 border-l-2 border-border/30 ml-3">
              <PlanMarkdownBody
                text={body}
                className="text-foreground/80 [&_strong]:text-foreground"
              />
            </div>
          </div>
        )}
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-[95vw] sm:max-w-[70vw] w-[95vw] max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          <PlanMarkdownBody text={body} />
        </DialogContent>
      </Dialog>
    </>
  );
});
