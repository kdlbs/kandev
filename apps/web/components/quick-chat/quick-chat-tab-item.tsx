"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { IconSparkles, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { GridSpinner } from "@/components/grid-spinner";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

type QuickChatTabItemProps = {
  name: string;
  isActive: boolean;
  isRenameable: boolean;
  isWorking?: boolean;
  kind?: QuickChatSessionKind;
  onActivate: () => void;
  onClose: () => void;
  onRename: (name: string) => void;
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
}: QuickChatTabItemProps) {
  const { t } = useTranslation();
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState(name);
  const inputRef = useRef<HTMLInputElement>(null);
  // Both Enter and Escape close edit mode by blurring the input so onBlur is
  // the single commit path. Escape additionally sets this ref so commit knows
  // to skip the rename — the blur fires synchronously with the typed draft
  // still in the closure, and we'd otherwise rename to whatever the user typed.
  const cancelledRef = useRef(false);

  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [isEditing]);

  const commit = useCallback(() => {
    if (cancelledRef.current) {
      cancelledRef.current = false;
      setIsEditing(false);
      return;
    }
    const trimmed = draft.trim();
    if (trimmed && trimmed !== name) onRename(trimmed);
    setIsEditing(false);
  }, [draft, name, onRename]);

  const cancel = useCallback(() => {
    cancelledRef.current = true;
    setDraft(name);
    inputRef.current?.blur();
  }, [name]);

  const handleStartEdit = useCallback(() => {
    if (!isRenameable) return;
    setDraft(name);
    setIsEditing(true);
  }, [isRenameable, name]);

  const tabContent = (
    <div
      data-testid="quick-chat-tab"
      className={`flex items-center gap-1 rounded transition-colors whitespace-nowrap ${
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:bg-muted"
      }`}
    >
      {isEditing ? (
        <input
          ref={inputRef}
          aria-label={t("chat:renameChat")}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            // IME composition uses Enter to confirm a candidate; let the IME
            // handle it instead of committing the rename.
            if (e.nativeEvent.isComposing) return;
            if (e.key === "Enter") {
              e.preventDefault();
              // Trigger blur instead of calling commit() directly: the input
              // unmount on setIsEditing(false) would otherwise fire onBlur and
              // call commit() a second time.
              inputRef.current?.blur();
            } else if (e.key === "Escape") {
              e.preventDefault();
              cancel();
            }
          }}
          className="px-2.5 py-1 text-xs bg-background border border-input rounded outline-none focus:ring-1 focus:ring-ring max-w-[160px]"
        />
      ) : (
        <button
          type="button"
          onClick={onActivate}
          onDoubleClick={handleStartEdit}
          title={isRenameable ? t("chat:doubleClickToRename") : undefined}
          className="flex items-center px-2.5 py-1 text-xs cursor-pointer"
        >
          {isWorking && <GridSpinner className="h-3 w-3 shrink-0 text-muted-foreground" />}
          {kind === "config" && (
            <span role="img" aria-label={t("chat:configurationChat")} className="mr-1.5 shrink-0">
              <IconSparkles className="h-3 w-3" aria-hidden />
            </span>
          )}
          <span className="truncate max-w-[160px]">{name}</span>
        </button>
      )}
      <button
        type="button"
        aria-label={t("chat:close", { name })}
        className="p-1 cursor-pointer opacity-60 hover:opacity-100"
        onClick={onClose}
      >
        <IconX className="h-3 w-3" />
      </button>
    </div>
  );

  if (!isRenameable) return tabContent;

  return <RenameContextMenu onRename={handleStartEdit}>{tabContent}</RenameContextMenu>;
});
