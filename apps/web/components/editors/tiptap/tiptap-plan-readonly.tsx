"use client";

import { useEffect } from "react";
import { useTheme } from "next-themes";
import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Highlight from "@tiptap/extension-highlight";
import Underline from "@tiptap/extension-underline";
import Link from "@tiptap/extension-link";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { Table } from "@tiptap/extension-table";
import { TableRow } from "@tiptap/extension-table-row";
import { TableCell } from "@tiptap/extension-table-cell";
import { TableHeader } from "@tiptap/extension-table-header";
import { Markdown } from "tiptap-markdown";
import { common, createLowlight } from "lowlight";
import { createCodeBlockWithMermaid } from "./tiptap-mermaid-extension";
import { cn } from "@/lib/utils";

type Props = {
  /** Markdown source. The editor's tiptap-markdown extension parses it via
   * `setContent`, so headings, lists, tables, and code blocks render properly. */
  content: string;
  className?: string;
  testId?: string;
};

const lowlight = createLowlight(common);

/** Read-only Tiptap renderer for plan revision content.
 *
 * Reuses the same StarterKit + Markdown + tables/tasks/code-block extensions as
 * the live plan editor so previewed markdown matches what the user sees while
 * editing — minus interactive bits (slash menu, drag handles, comment marks).
 *
 * Initial markdown content is set via `editor.commands.setContent(content, ...)`
 * after creation rather than the `content` option, because tiptap-markdown's
 * setContent override is what triggers the markdown -> doc parse.
 */
export function PlanReadOnlyMarkdown({ content, className, testId }: Props) {
  const { resolvedTheme } = useTheme();
  const editor = useEditor({
    immediatelyRender: false,
    editable: false,
    extensions: [
      StarterKit.configure({ codeBlock: false }),
      createCodeBlockWithMermaid(lowlight),
      Markdown.configure({ html: true, transformPastedText: false, transformCopiedText: false }),
      Link.configure({ openOnClick: false }),
      Highlight,
      Underline,
      TaskList,
      TaskItem.configure({ nested: true }),
      Table.configure({ resizable: false }),
      TableRow,
      TableCell,
      TableHeader,
    ],
  });

  useEffect(() => {
    if (!editor || editor.isDestroyed) return;
    // Tiptap-markdown intercepts setContent for markdown strings; setting
    // content this way (instead of via the `content` option) ensures the
    // input gets parsed as markdown rather than treated as raw text/HTML.
    editor.commands.setContent(content, { emitUpdate: false });
  }, [editor, content]);

  // Reuse the same class names as the live plan editor so the heading sizes,
  // list spacing, code-block styling, etc. defined in `globals.css` under
  // `.markdown-body` and `.tiptap-plan-wrapper` apply identically here.
  return (
    <EditorContent
      editor={editor}
      data-testid={testId}
      className={cn(
        "tiptap-plan-wrapper markdown-body !h-auto",
        resolvedTheme === "dark" && "dark",
        "[&_.ProseMirror]:outline-none [&_.ProseMirror]:focus:outline-none",
        // Trim the editor's vertical padding for the more compact preview body.
        "[&_.tiptap]:!p-0",
        className,
      )}
    />
  );
}
