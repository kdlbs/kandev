"use client";

import { useCallback, useEffect, useState, type ElementType, type ReactNode } from "react";
import { createPortal } from "react-dom";
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
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  resolveVisualViewportPosition,
  useVisualViewportOffset,
} from "@/hooks/use-visual-viewport-offset";

export const PLAN_FORMATTING_TOOLBAR_HEIGHT_PX = 56;

type PlanBubbleMenuProps = {
  editor: Editor;
  onComment?: (selection: TextSelection) => void;
  /** Internal mobile layout offset. It is not part of the plugin editor contract. */
  mobileBottomOffset?: string;
  onMobileVisibilityChange?: (visible: boolean) => void;
};

type EditorSnapshot = {
  isFocused: boolean;
  isCodeBlock: boolean;
  hasCommentSelection: boolean;
  isBold: boolean;
  isItalic: boolean;
  isUnderline: boolean;
  isStrike: boolean;
  isCode: boolean;
  isHighlight: boolean;
  isLink: boolean;
};

function readEditorSnapshot(editor: Editor): EditorSnapshot {
  const { from, to } = editor.state.selection;
  const selectedText = editor.state.doc.textBetween(from, to, " ").trim();
  return {
    isFocused: editor.isFocused,
    isCodeBlock: editor.isActive("codeBlock"),
    hasCommentSelection: from !== to && selectedText.length > 0,
    isBold: editor.isActive("bold"),
    isItalic: editor.isActive("italic"),
    isUnderline: editor.isActive("underline"),
    isStrike: editor.isActive("strike"),
    isCode: editor.isActive("code"),
    isHighlight: editor.isActive("highlight"),
    isLink: editor.isActive("link"),
  };
}

function useEditorSnapshot(editor: Editor): EditorSnapshot {
  const [snapshot, setSnapshot] = useState(() => readEditorSnapshot(editor));

  useEffect(() => {
    const update = () => setSnapshot(readEditorSnapshot(editor));
    editor.on("transaction", update);
    editor.on("selectionUpdate", update);
    editor.on("focus", update);
    editor.on("blur", update);
    update();
    return () => {
      editor.off("transaction", update);
      editor.off("selectionUpdate", update);
      editor.off("focus", update);
      editor.off("blur", update);
    };
  }, [editor]);

  return snapshot;
}

type ToggleButtonProps = {
  icon: ElementType;
  isActive: boolean;
  onClick: () => void;
  title: string;
  testId: string;
  accent?: boolean;
  disabled?: boolean;
  docked?: boolean;
  toggle?: boolean;
};

function ToggleButton({
  icon: Icon,
  isActive,
  onClick,
  title,
  testId,
  accent,
  disabled = false,
  docked = false,
  toggle = true,
}: ToggleButtonProps) {
  return (
    <button
      type="button"
      title={title}
      aria-label={title}
      aria-pressed={toggle ? isActive : undefined}
      disabled={disabled}
      data-testid={testId}
      className={cn(
        "rounded cursor-pointer transition-colors shrink-0",
        docked ? "h-11 min-w-11 p-2" : "p-1.5",
        accent ? "bg-primary text-primary-foreground hover:bg-primary/90" : "hover:bg-muted/80",
        isActive && "bg-muted text-primary ring-1 ring-inset ring-primary/50",
        disabled && "cursor-not-allowed opacity-50",
      )}
      onPointerDown={(e) => e.preventDefault()}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
    >
      <Icon className={cn(docked ? "h-5 w-5" : "h-3.5 w-3.5")} aria-hidden="true" />
    </button>
  );
}

function MenuSeparator({ docked = false }: { docked?: boolean }) {
  return <div className={cn("w-px bg-border/50 mx-0.5", docked ? "h-7" : "h-5")} />;
}

function LinkInput({
  onSubmit,
  onCancel,
  docked = false,
}: {
  onSubmit: (url: string) => void;
  onCancel: () => void;
  docked?: boolean;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");
  const linkLabel = t("editors:pasteLink");
  return (
    <div
      className={cn(
        "flex items-center gap-1 bg-popover border border-border/50 rounded-lg shadow-lg p-1",
        docked && "min-w-max",
      )}
    >
      <input
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") onSubmit(value);
          if (e.key === "Escape") onCancel();
        }}
        placeholder={linkLabel}
        aria-label={linkLabel}
        className={cn("text-xs bg-transparent outline-none px-2 py-1", docked ? "w-40" : "w-48")}
        autoFocus
      />
      <button
        type="button"
        aria-label={t("editors:apply")}
        className={cn(
          "rounded hover:bg-muted cursor-pointer shrink-0",
          docked ? "h-11 min-w-11 px-2" : "text-xs px-2 py-1",
        )}
        onPointerDown={(e) => e.preventDefault()}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => onSubmit(value)}
      >
        {t("editors:apply")}
      </button>
    </div>
  );
}

function FormatToolbar({
  editor,
  snapshot,
  onLinkClick,
  onComment,
  docked = false,
}: {
  editor: Editor;
  snapshot: EditorSnapshot;
  onLinkClick: () => void;
  onComment?: () => void;
  docked?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        "flex items-center gap-0.5",
        !docked && "bg-popover border border-border/50 rounded-lg shadow-lg p-1",
      )}
    >
      <ToggleButton
        icon={IconBold}
        title={t("editors:boldCmdB")}
        testId="plan-formatting-action-bold"
        isActive={snapshot.isBold}
        docked={docked}
        onClick={() => editor.chain().focus().toggleBold().run()}
      />
      <ToggleButton
        icon={IconItalic}
        title={t("editors:italicCmdI")}
        testId="plan-formatting-action-italic"
        isActive={snapshot.isItalic}
        docked={docked}
        onClick={() => editor.chain().focus().toggleItalic().run()}
      />
      <ToggleButton
        icon={IconUnderline}
        title={t("editors:underlineCmdU")}
        testId="plan-formatting-action-underline"
        isActive={snapshot.isUnderline}
        docked={docked}
        onClick={() => editor.chain().focus().toggleUnderline().run()}
      />
      <ToggleButton
        icon={IconStrikethrough}
        title={t("editors:strikethroughCmdShiftX")}
        testId="plan-formatting-action-strike"
        isActive={snapshot.isStrike}
        docked={docked}
        onClick={() => editor.chain().focus().toggleStrike().run()}
      />
      <MenuSeparator docked={docked} />
      <ToggleButton
        icon={IconCode}
        title={t("editors:inlineCode")}
        testId="plan-formatting-action-code"
        isActive={snapshot.isCode}
        docked={docked}
        onClick={() => editor.chain().focus().toggleCode().run()}
      />
      <ToggleButton
        icon={IconHighlight}
        title={t("editors:highlight")}
        testId="plan-formatting-action-highlight"
        isActive={snapshot.isHighlight}
        docked={docked}
        onClick={() => editor.chain().focus().toggleHighlight().run()}
      />
      <ToggleButton
        icon={IconLink}
        title={t("editors:link")}
        testId="plan-formatting-action-link"
        isActive={snapshot.isLink}
        docked={docked}
        onClick={onLinkClick}
      />
      {onComment && (
        <>
          <MenuSeparator docked={docked} />
          <ToggleButton
            icon={IconMessage}
            title={t("editors:commentCmdShiftC")}
            testId="plan-formatting-action-comment"
            isActive={false}
            toggle={false}
            disabled={!snapshot.hasCommentSelection}
            docked={docked}
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

function MobileFormattingToolbar({
  content,
  label,
  keyboardOpen,
  viewportBottom,
  mobileBottomOffset,
}: {
  content: ReactNode;
  label: string;
  keyboardOpen: boolean;
  viewportBottom: number;
  mobileBottomOffset?: string;
}) {
  if (typeof document === "undefined") return null;
  const position = resolveVisualViewportPosition({
    keyboardOpen,
    viewportBottom,
    barHeight: PLAN_FORMATTING_TOOLBAR_HEIGHT_PX,
    baseBottomOffset: mobileBottomOffset,
  });
  return createPortal(
    <div
      data-testid="plan-mobile-formatting-toolbar"
      role="toolbar"
      aria-label={label}
      className="fixed left-0 right-0 z-40 border-t border-border bg-popover/95 backdrop-blur"
      style={{ ...position, height: `${PLAN_FORMATTING_TOOLBAR_HEIGHT_PX}px` }}
    >
      <div className="flex h-full w-full min-w-0 items-center gap-1 overflow-x-auto overscroll-x-contain px-2 py-1">
        {content}
      </div>
    </div>,
    document.body,
  );
}

export function PlanBubbleMenu({
  editor,
  onComment,
  mobileBottomOffset,
  onMobileVisibilityChange,
}: PlanBubbleMenuProps) {
  const { t } = useTranslation();
  const { isMobile, isTablet, usesDesktopWorkbench } = useResponsiveBreakpoint();
  const { keyboardOpen, viewportBottom } = useVisualViewportOffset();
  const [showLinkInput, setShowLinkInput] = useState(false);
  const snapshot = useEditorSnapshot(editor);
  const isMobilePresentation = isMobile || isTablet || !usesDesktopWorkbench;
  const mobileVisible =
    isMobilePresentation && (snapshot.isFocused || showLinkInput) && !snapshot.isCodeBlock;

  useEffect(() => {
    onMobileVisibilityChange?.(mobileVisible);
  }, [mobileVisible, onMobileVisibilityChange]);

  useEffect(
    () => () => {
      onMobileVisibilityChange?.(false);
    },
    [onMobileVisibilityChange],
  );

  const handleLinkClick = useCallback(() => {
    const existing = editor.getAttributes("link").href as string | undefined;
    if (existing) {
      editor.chain().focus().unsetLink().run();
    } else {
      setShowLinkInput(true);
    }
  }, [editor]);

  const focusEditor = useCallback(() => {
    editor.commands.focus();
  }, [editor]);

  const handleLinkSubmit = useCallback(
    (url: string) => {
      if (url.trim()) {
        editor.chain().focus().setLink({ href: url.trim() }).run();
      } else {
        focusEditor();
      }
      setShowLinkInput(false);
    },
    [editor, focusEditor],
  );

  const handleLinkCancel = useCallback(() => {
    setShowLinkInput(false);
    focusEditor();
  }, [focusEditor]);

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

  const content = showLinkInput ? (
    <LinkInput
      onSubmit={handleLinkSubmit}
      onCancel={handleLinkCancel}
      docked={isMobilePresentation}
    />
  ) : (
    <FormatToolbar
      editor={editor}
      snapshot={snapshot}
      onLinkClick={handleLinkClick}
      onComment={onComment ? handleComment : undefined}
      docked={isMobilePresentation}
    />
  );

  if (isMobilePresentation) {
    if (!mobileVisible) return null;
    return (
      <MobileFormattingToolbar
        content={content}
        label={t("editors:planFormattingToolbar")}
        keyboardOpen={keyboardOpen}
        viewportBottom={viewportBottom}
        mobileBottomOffset={mobileBottomOffset}
      />
    );
  }

  return (
    <BubbleMenu editor={editor} options={{ placement: "top" }} shouldShow={shouldShowBubbleMenu}>
      {content}
    </BubbleMenu>
  );
}
