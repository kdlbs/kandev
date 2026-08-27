"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import type { ChangeEvent, FocusEvent, KeyboardEvent, ReactNode, RefObject } from "react";
import {
  IconChevronLeft,
  IconChevronRight,
  IconDots,
  IconSparkles,
  IconX,
} from "@tabler/icons-react";
import type { DraggableAttributes, DraggableSyntheticListeners } from "@dnd-kit/core";
import { useTranslation } from "react-i18next";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { GridSpinner } from "@/components/grid-spinner";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export type QuickChatTabDragProps = {
  attributes: DraggableAttributes;
  listeners: DraggableSyntheticListeners;
  setActivatorNodeRef: (node: HTMLElement | null) => void;
};

export type QuickChatTabActionMenuProps = {
  name: string;
  closeLabel?: string;
  isRenameable?: boolean;
  onRename?: () => void;
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
  canMoveLeft?: boolean;
  canMoveRight?: boolean;
  onClose: () => void;
};

/** Coarse-pointer action path for tab operations that cannot depend on hover or long-press. */
export function QuickChatTabActionMenu({
  name,
  closeLabel,
  isRenameable = false,
  onRename,
  onMoveLeft,
  onMoveRight,
  canMoveLeft = true,
  canMoveRight = true,
  onClose,
}: QuickChatTabActionMenuProps) {
  const { t } = useTranslation();
  const hasMoveActions = Boolean(onMoveLeft && onMoveRight);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={t("chat:quickChatTabActions", { name })}
          title={t("chat:quickChatTabActions", { name })}
          className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-70 hover:opacity-100"
        >
          <IconDots className="h-4 w-4" aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {isRenameable && onRename && (
          <DropdownMenuItem className="min-h-11 cursor-pointer sm:min-h-7" onSelect={onRename}>
            {t("common:rename")}
          </DropdownMenuItem>
        )}
        {hasMoveActions && (
          <>
            <DropdownMenuItem
              className="min-h-11 cursor-pointer sm:min-h-7"
              disabled={!canMoveLeft}
              onSelect={onMoveLeft}
            >
              {t("chat:moveQuickChatTabLeft", { name })}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="min-h-11 cursor-pointer sm:min-h-7"
              disabled={!canMoveRight}
              onSelect={onMoveRight}
            >
              {t("chat:moveQuickChatTabRight", { name })}
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuItem className="min-h-11 cursor-pointer sm:min-h-7" onSelect={onClose}>
          {closeLabel ?? t("chat:close", { name })}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export type QuickChatTabMoveButtonsProps = {
  name: string;
  onMoveLeft: () => void;
  onMoveRight: () => void;
  canMoveLeft: boolean;
  canMoveRight: boolean;
};

export function QuickChatTabMoveButtons({
  name,
  onMoveLeft,
  onMoveRight,
  canMoveLeft,
  canMoveRight,
}: QuickChatTabMoveButtonsProps) {
  const { t } = useTranslation();

  return (
    <>
      <button
        type="button"
        aria-label={t("chat:moveQuickChatTabLeft", { name })}
        className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-70 hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-30 sm:hidden"
        disabled={!canMoveLeft}
        onClick={onMoveLeft}
      >
        <IconChevronLeft className="h-4 w-4" aria-hidden />
      </button>
      <button
        type="button"
        aria-label={t("chat:moveQuickChatTabRight", { name })}
        className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-70 hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-30 sm:hidden"
        disabled={!canMoveRight}
        onClick={onMoveRight}
      >
        <IconChevronRight className="h-4 w-4" aria-hidden />
      </button>
    </>
  );
}

type QuickChatTabItemProps = {
  name: string;
  isActive: boolean;
  isRenameable: boolean;
  isWorking?: boolean;
  kind?: QuickChatSessionKind;
  onActivate: () => void;
  onClose: () => void;
  onRename: (name: string) => void;
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
  canMoveLeft?: boolean;
  canMoveRight?: boolean;
  dragProps?: QuickChatTabDragProps;
};

function RenameContextMenu({ children, onRename }: { children: ReactNode; onRename: () => void }) {
  const { t } = useTranslation();

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem className="min-h-11 cursor-pointer sm:min-h-7" onSelect={onRename}>
          {t("common:rename")}
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

type QuickChatTabBodyProps = {
  name: string;
  kind: QuickChatSessionKind;
  isWorking: boolean;
  isEditing: boolean;
  isRenameable: boolean;
  draft: string;
  inputRef: RefObject<HTMLInputElement | null>;
  onDraftChange: (event: ChangeEvent<HTMLInputElement>) => void;
  onBlur: (event: FocusEvent<HTMLInputElement>) => void;
  onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => void;
  onActivate: () => void;
  onStartEdit: () => void;
};

function QuickChatTabBody({
  name,
  kind,
  isWorking,
  isEditing,
  isRenameable,
  draft,
  inputRef,
  onDraftChange,
  onBlur,
  onKeyDown,
  onActivate,
  onStartEdit,
}: QuickChatTabBodyProps) {
  const { t } = useTranslation();

  if (isEditing) {
    return (
      <input
        ref={inputRef}
        aria-label={t("chat:renameChat")}
        value={draft}
        onChange={onDraftChange}
        onBlur={onBlur}
        onKeyDown={onKeyDown}
        className="h-11 max-w-[160px] rounded border border-primary bg-accent/60 px-2.5 py-1 text-base outline-none ring-2 ring-primary/30 focus:border-ring focus:ring-2 focus:ring-ring sm:h-6 sm:text-xs"
      />
    );
  }

  return (
    <button
      type="button"
      onClick={onActivate}
      onDoubleClick={onStartEdit}
      title={isRenameable ? t("chat:doubleClickToRename") : undefined}
      className="flex h-11 cursor-pointer items-center gap-1.5 px-2.5 py-1 text-xs sm:h-6"
    >
      {isWorking && <GridSpinner className="h-3 w-3 shrink-0 text-muted-foreground" />}
      {kind === "config" && (
        <span role="img" aria-label={t("chat:configurationChat")} className="shrink-0">
          <IconSparkles className="h-3 w-3" aria-hidden />
        </span>
      )}
      <span data-testid="quick-chat-tab-name" className="max-w-[160px] truncate">
        {name}
      </span>
    </button>
  );
}

type QuickChatTabActionsProps = {
  name: string;
  isEditing: boolean;
  isRenameable: boolean;
  onStartEdit: () => void;
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
  canMoveLeft: boolean;
  canMoveRight: boolean;
  onClose: () => void;
};

function QuickChatTabActions({
  name,
  isEditing,
  isRenameable,
  onStartEdit,
  onMoveLeft,
  onMoveRight,
  canMoveLeft,
  canMoveRight,
  onClose,
}: QuickChatTabActionsProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();

  if (isEditing) return null;

  return (
    <div className="flex shrink-0 items-center">
      {!isFinePointer && (
        <QuickChatTabActionMenu
          name={name}
          isRenameable={isRenameable}
          onRename={onStartEdit}
          onMoveLeft={onMoveLeft}
          onMoveRight={onMoveRight}
          canMoveLeft={canMoveLeft}
          canMoveRight={canMoveRight}
          onClose={onClose}
        />
      )}
      {isFinePointer && onMoveLeft && onMoveRight && (
        <QuickChatTabMoveButtons
          name={name}
          onMoveLeft={onMoveLeft}
          onMoveRight={onMoveRight}
          canMoveLeft={canMoveLeft}
          canMoveRight={canMoveRight}
        />
      )}
      {isFinePointer && (
        <button
          type="button"
          aria-label={t("chat:close", { name })}
          className="flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center opacity-60 hover:opacity-100 sm:h-6 sm:w-6"
          onClick={onClose}
        >
          <IconX className="h-3 w-3" aria-hidden />
        </button>
      )}
    </div>
  );
}

type QuickChatTabContentProps = {
  isActive: boolean;
  editContainerRef: RefObject<HTMLDivElement | null>;
  bodyProps: QuickChatTabBodyProps;
  actionsProps: QuickChatTabActionsProps;
  dragProps?: QuickChatTabDragProps;
};

function QuickChatTabContent({
  isActive,
  editContainerRef,
  bodyProps,
  actionsProps,
  dragProps,
}: QuickChatTabContentProps) {
  let tabStateClassName = "text-muted-foreground hover:bg-muted";
  if (bodyProps.isEditing) {
    tabStateClassName =
      "border border-primary bg-accent/40 text-foreground shadow-sm ring-1 ring-primary/30";
  } else if (isActive) {
    tabStateClassName = "bg-background text-foreground shadow-sm";
  }

  const content = (
    <div
      ref={bodyProps.isEditing ? editContainerRef : dragProps?.setActivatorNodeRef}
      {...(bodyProps.isEditing ? {} : (dragProps?.attributes ?? {}))}
      {...(bodyProps.isEditing ? {} : (dragProps?.listeners ?? {}))}
      data-testid="quick-chat-tab"
      className={`flex items-center gap-1 rounded whitespace-nowrap transition-colors ${tabStateClassName} ${
        dragProps && !bodyProps.isEditing ? "cursor-grab active:cursor-grabbing" : ""
      }`}
    >
      <div className="flex items-center">
        <QuickChatTabBody {...bodyProps} />
      </div>
      <QuickChatTabActions {...actionsProps} />
    </div>
  );

  if (!bodyProps.isRenameable) return content;
  return <RenameContextMenu onRename={bodyProps.onStartEdit}>{content}</RenameContextMenu>;
}

function useQuickChatTabRename({
  name,
  isRenameable,
  onRename,
}: Pick<QuickChatTabItemProps, "name" | "isRenameable" | "onRename">) {
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState(name);
  const inputRef = useRef<HTMLInputElement>(null);
  const editContainerRef = useRef<HTMLDivElement>(null);
  // Blur, Enter, and Escape can all finish the same edit. Guard
  // the transition so one edit can produce at most one rename or restore.
  const editResultRef = useRef<"open" | "committed" | "cancelled">("open");

  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [isEditing]);

  const commit = useCallback(() => {
    if (editResultRef.current !== "open") return;
    editResultRef.current = "committed";
    const trimmed = draft.trim();
    if (trimmed && trimmed !== name) onRename(trimmed);
    setIsEditing(false);
  }, [draft, name, onRename]);

  const cancel = useCallback(() => {
    if (editResultRef.current !== "open") return;
    editResultRef.current = "cancelled";
    setDraft(name);
    setIsEditing(false);
  }, [name]);

  const handleStartEdit = useCallback(() => {
    if (!isRenameable) return;
    editResultRef.current = "open";
    setDraft(name);
    setIsEditing(true);
  }, [isRenameable, name]);

  const handleInputBlur = useCallback(
    (event: FocusEvent<HTMLInputElement>) => {
      if (
        event.relatedTarget instanceof Node &&
        editContainerRef.current?.contains(event.relatedTarget)
      ) {
        return;
      }
      commit();
    },
    [commit],
  );

  const handleDraftChange = useCallback((event: ChangeEvent<HTMLInputElement>) => {
    setDraft(event.target.value);
  }, []);

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      // IME composition uses Enter to confirm a candidate; let the IME
      // handle it instead of committing the rename.
      if (event.nativeEvent.isComposing) return;
      if (event.key === "Enter") {
        event.preventDefault();
        commit();
      } else if (event.key === "Escape") {
        event.preventDefault();
        cancel();
      }
    },
    [cancel, commit],
  );

  return {
    isEditing,
    draft,
    inputRef,
    editContainerRef,
    handleStartEdit,
    handleInputBlur,
    handleDraftChange,
    handleKeyDown,
  };
}

/** Tab in the quick-chat modal. Renameable tabs support double-click and context-menu editing. */
export const QuickChatTabItem = memo(function QuickChatTabItem({
  name,
  isActive,
  isRenameable,
  isWorking = false,
  kind = "chat",
  onActivate,
  onClose,
  onRename,
  onMoveLeft,
  onMoveRight,
  canMoveLeft = true,
  canMoveRight = true,
  dragProps,
}: QuickChatTabItemProps) {
  const {
    isEditing,
    draft,
    inputRef,
    editContainerRef,
    handleStartEdit,
    handleInputBlur,
    handleDraftChange,
    handleKeyDown,
  } = useQuickChatTabRename({ name, isRenameable, onRename });

  return (
    <QuickChatTabContent
      isActive={isActive}
      editContainerRef={editContainerRef}
      bodyProps={{
        name,
        kind,
        isWorking,
        isEditing,
        isRenameable,
        draft,
        inputRef,
        onDraftChange: handleDraftChange,
        onBlur: handleInputBlur,
        onKeyDown: handleKeyDown,
        onActivate,
        onStartEdit: handleStartEdit,
      }}
      actionsProps={{
        name,
        isEditing,
        isRenameable,
        onStartEdit: handleStartEdit,
        onMoveLeft,
        onMoveRight,
        canMoveLeft,
        canMoveRight,
        onClose,
      }}
      dragProps={dragProps}
    />
  );
});
