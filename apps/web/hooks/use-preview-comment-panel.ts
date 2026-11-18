"use client";

import { useCallback, useState } from "react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useLocalStorageBoolean } from "@/hooks/use-local-storage-boolean";

export const PREVIEW_COMMENT_PANEL_KEY = "kandev:preview-comment-panel-open";
const PREVIEW_COMMENT_PANEL_EVENT = "kandev:preview-comment-panel-change";

/**
 * Whether the annotation comment panel is shown. Fine pointers remember the
 * choice per device and start open; coarse pointers use a bottom sheet that
 * starts hidden on every entry.
 */
export function usePreviewCommentPanel() {
  const touch = useTouchDrawer();
  const stored = useLocalStorageBoolean(
    PREVIEW_COMMENT_PANEL_KEY,
    PREVIEW_COMMENT_PANEL_EVENT,
    true,
  );
  const [sheetOpen, setSheetOpen] = useState(false);
  const open = touch ? sheetOpen : stored.value;
  const { setValue } = stored;
  const setOpen = useCallback(
    (next: boolean) => (touch ? setSheetOpen(next) : setValue(next)),
    [setValue, touch],
  );
  return {
    open,
    setOpen,
    toggle: useCallback(() => setOpen(!open), [open, setOpen]),
    touch,
    /** Shown beside or over the preview, rather than as the touch sheet. */
    docked: !touch && open,
  };
}
