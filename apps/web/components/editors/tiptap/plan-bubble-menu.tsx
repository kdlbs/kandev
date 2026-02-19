"use client";

import { useCallback, useState } from "react";
import { BubbleMenu } from "@tiptap/react/menus";
import type { Editor } from "@tiptap/core";
import type { EditorState } from "@tiptap/pm/state";
import {
  IconBold,
  IconItalic,
  IconUnderline,
  IconStrikethrough,
  IconCode,
  IconHighlight,
  IconLink,
  IconMessage,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { TextSelection } from "./tiptap-plan-editor";

type PlanBubbleMenuProps = {
  editor: Editor;
  onComment?: (selection: TextSelection) => void;
};

type ToggleButtonProps = {
  icon: React.ElementType;
  isActive: boolean;
  onClick: () => void;
  title: string;
  accent?: boolean;
};

function ToggleButton({ icon: Icon, isActive, onClick, title, accent }: ToggleButtonProps) {
  return (
    <button
      type="button"
      title={title}
      className={cn(
        "p-1.5 rounded cursor-pointer transition-colors",
        accent ? "text-white bg-accent hover:bg-accent/80" : "hover:bg-muted/80",
        isActive && "bg-muted text-primary",
      )}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  );
}

function MenuSeparator() {
  return <div className="w-px h-5 bg-border/50 mx-0.5" />;
}

function LinkInput({
  onSubmit,
  onCancel,
}: {
  onSubmit: (url: string) => void;
  onCancel: () => void;
}) {
  const [value, setValue] = useState("");
  return (
    <div className="flex items-center gap-1 bg-popover border border-border/50 rounded-lg shadow-lg p-1">
      <input
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") onSubmit(value);
          if (e.key === "Escape") onCancel();
        }}
        placeholder="Paste link..."
        className="text-xs bg-transparent outline-none px-2 py-1 w-48"
        autoFocus
      />
      <button
        type="button"
        className="text-xs px-2 py-1 rounded hover:bg-muted cursor-pointer"
        onClick={() => onSubmit(value)}
      >
        Apply
      </button>
    </div>
  );
}

function FormatToolbar({
  editor,
  onLinkClick,
  onComment,
}: {
  editor: Editor;
  onLinkClick: () => void;
  onComment?: () => void;
}) {
  return (
    <div className="flex items-center gap-0.5 bg-popover border border-border/50 rounded-lg shadow-lg p-1">
      <ToggleButton
        icon={IconBold}
        title="Bold (Cmd+B)"
        isActive={editor.isActive("bold")}
        onClick={() => editor.chain().focus().toggleBold().run()}
      />
      <ToggleButton
        icon={IconItalic}
        title="Italic (Cmd+I)"
        isActive={editor.isActive("italic")}
        onClick={() => editor.chain().focus().toggleItalic().run()}
      />
      <ToggleButton
        icon={IconUnderline}
        title="Underline (Cmd+U)"
        isActive={editor.isActive("underline")}
        onClick={() => editor.chain().focus().toggleUnderline().run()}
      />
      <ToggleButton
        icon={IconStrikethrough}
        title="Strikethrough (Cmd+Shift+X)"
        isActive={editor.isActive("strike")}
        onClick={() => editor.chain().focus().toggleStrike().run()}
      />
      <MenuSeparator />
      <ToggleButton
        icon={IconCode}
        title="Inline code"
        isActive={editor.isActive("code")}
        onClick={() => editor.chain().focus().toggleCode().run()}
      />
      <ToggleButton
        icon={IconHighlight}
        title="Highlight"
        isActive={editor.isActive("highlight")}
        onClick={() => editor.chain().focus().toggleHighlight().run()}
      />
      <ToggleButton
        icon={IconLink}
        title="Link"
        isActive={editor.isActive("link")}
        onClick={onLinkClick}
      />
      {onComment && (
        <>
          <MenuSeparator />
          <ToggleButton
            icon={IconMessage}
            title="Comment (Cmd+Shift+C)"
            isActive={false}
            onClick={onComment}
            accent
          />
        </>
      )}
    </div>
  );
}

function shouldShowBubbleMenu({
  editor: ed,
  state,
}: {
  editor: Editor;
  state: EditorState;
}): boolean {
  const { from, to } = state.selection;
  if (from === to) return false;
  if (ed.isActive("codeBlock")) return false;
  return true;
}

export function PlanBubbleMenu({ editor, onComment }: PlanBubbleMenuProps) {
  const [showLinkInput, setShowLinkInput] = useState(false);

  const handleLinkClick = useCallback(() => {
    const existing = editor.getAttributes("link").href as string | undefined;
    if (existing) {
      editor.chain().focus().unsetLink().run();
    } else {
      setShowLinkInput(true);
    }
  }, [editor]);

  const handleLinkSubmit = useCallback(
    (url: string) => {
      if (url.trim()) {
        editor.chain().focus().setLink({ href: url.trim() }).run();
      }
      setShowLinkInput(false);
    },
    [editor],
  );

  const handleComment = useCallback(() => {
    if (!onComment) return;
    const { from, to } = editor.state.selection;
    const text = editor.state.doc.textBetween(from, to, " ");
    if (!text.trim()) return;
    const endCoords = editor.view.coordsAtPos(to);
    onComment({
      text: text.trim(),
      from,
      to,
      position: { x: endCoords.left, y: endCoords.bottom },
    });
  }, [editor, onComment]);

  return (
    <BubbleMenu editor={editor} options={{ placement: "top" }} shouldShow={shouldShowBubbleMenu}>
      {showLinkInput ? (
        <LinkInput onSubmit={handleLinkSubmit} onCancel={() => setShowLinkInput(false)} />
      ) : (
        <FormatToolbar
          editor={editor}
          onLinkClick={handleLinkClick}
          onComment={onComment ? handleComment : undefined}
        />
      )}
    </BubbleMenu>
  );
}
