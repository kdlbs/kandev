"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { remarkPlugins, markdownComponents } from "@/components/shared/markdown-components";

type ContentMode = "view" | "code" | "edit";

interface SkillContentProps {
  content: string;
  mode: ContentMode;
  onContentChange?: (content: string) => void;
  onSave?: (content: string) => void;
  onCancel?: () => void;
}

/** Strip YAML frontmatter (---\n...\n---) from the start of content. */
function stripFrontmatter(content: string): string {
  const match = content.match(/^---\n[\s\S]*?\n---\n?/);
  return match ? content.slice(match[0].length) : content;
}

export function SkillContent({
  content,
  mode,
  onContentChange,
  onSave,
  onCancel,
}: SkillContentProps) {
  const [draft, setDraft] = useState(content);

  if (mode === "edit") {
    return (
      <div className="space-y-3">
        <Textarea
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            onContentChange?.(e.target.value);
          }}
          className="font-mono text-sm min-h-[400px]"
          placeholder="# Skill instructions..."
        />
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={onCancel}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={() => onSave?.(draft)}
            className="cursor-pointer"
          >
            Save
          </Button>
        </div>
      </div>
    );
  }

  if (mode === "code") {
    return (
      <pre className="text-sm font-mono bg-muted p-4 rounded-lg overflow-x-auto whitespace-pre-wrap">
        {content || "No content"}
      </pre>
    );
  }

  // view mode - rendered markdown
  return (
    <div className="markdown-body max-w-3xl">
      <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {content ? stripFrontmatter(content) : "*No content*"}
      </ReactMarkdown>
    </div>
  );
}
