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
import { useTranslation } from "react-i18next";

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
        accent ? "bg-primary text-primary-foreground hover:bg-primary/90" : "hover:bg-muted/80",
        isActive && "bg-muted text-primary ring-1 ring-inset ring-primary/50",
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
  const { t } = useTranslation();
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
        placeholder={t("editors:pasteLink")}
        className="text-xs bg-transparent outline-none px-2 py-1 w-48"
        autoFocus
      />
      <button
        type="button"
        className="text-xs px-2 py-1 rounded hover:bg-muted cursor-pointer"
        onClick={() => onSubmit(value)}
      >
        {t("editors:apply")}
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
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-0.5 bg-popover border border-border/50 rounded-lg shadow-lg p-1">
      <ToggleButton
        icon={IconBold}
        title={t("editors:boldCmdB")}
        isActive={editor.isActive("bold")}
        onClick={() => editor.chain().focus().toggleBold().run()}
      />
      <ToggleButton
        icon={IconItalic}
        title={t("editors:italicCmdI")}
        isActive={editor.isActive("italic")}
        onClick={() => editor.chain().focus().toggleItalic().run()}
      />
      <ToggleButton
        icon={IconUnderline}
        title={t("editors:underlineCmdU")}
        isActive={editor.isActive("underline")}
        onClick={() => editor.chain().focus().toggleUnderline().run()}
      />
      <ToggleButton
        icon={IconStrikethrough}
        title={t("editors:strikethroughCmdShiftX")}
        isActive={editor.isActive("strike")}
        onClick={() => editor.chain().focus().toggleStrike().run()}
      />
      <MenuSeparator />
      <ToggleButton
        icon={IconCode}
        title={t("editors:inlineCode")}
        isActive={editor.isActive("code")}
        onClick={() => editor.chain().focus().toggleCode().run()}
      />
      <ToggleButton
        icon={IconHighlight}
        title={t("editors:highlight")}
        isActive={editor.isActive("highlight")}
        onClick={() => editor.chain().focus().toggleHighlight().run()}
      />
      <ToggleButton
        icon={IconLink}
        title={t("editors:link")}
        isActive={editor.isActive("link")}
        onClick={onLinkClick}
      />
      {onComment && (
        <>
          <MenuSeparator />
          <ToggleButton
            icon={IconMessage}
            title={t("editors:commentCmdShiftC")}
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
