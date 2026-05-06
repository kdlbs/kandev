"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { IconX } from "@tabler/icons-react";

type QuickChatTabItemProps = {
  name: string;
  isActive: boolean;
  isRenameable: boolean;
  onActivate: () => void;
  onClose: () => void;
  onRename: (name: string) => void;
};

/** Tab in the quick-chat modal. Double-click the label to rename (local-only). */
export const QuickChatTabItem = memo(function QuickChatTabItem({
  name,
  isActive,
  isRenameable,
  onActivate,
  onClose,
  onRename,
}: QuickChatTabItemProps) {
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

  return (
    <div
      className={`flex items-center gap-1 rounded transition-colors whitespace-nowrap ${
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:bg-muted"
      }`}
    >
      {isEditing ? (
        <input
          ref={inputRef}
          aria-label="Rename chat"
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
          title={isRenameable ? "Double-click to rename" : undefined}
          className="flex items-center px-2.5 py-1 text-xs cursor-pointer"
        >
          <span className="truncate max-w-[160px]">{name}</span>
        </button>
      )}
      <button
        type="button"
        aria-label={`Close ${name}`}
        className="p-1 cursor-pointer opacity-60 hover:opacity-100"
        onClick={onClose}
      >
        <IconX className="h-3 w-3" />
      </button>
    </div>
  );
});
