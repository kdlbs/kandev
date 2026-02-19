"use client";

import type { ReactNode } from "react";
import { useCallback, useEffect, useRef } from "react";

type EditableCardRenderArgs = {
  open: () => void;
  close: () => void;
};

type EditableCardProps = {
  isEditing: boolean;
  historyId: string;
  onOpen: () => void;
  onClose: () => void;
  renderPreview: (args: Pick<EditableCardRenderArgs, "open">) => ReactNode;
  renderEdit: (args: Pick<EditableCardRenderArgs, "close">) => ReactNode;
};

export function EditableCard({
  isEditing,
  historyId,
  onOpen,
  onClose,
  renderPreview,
  renderEdit,
}: EditableCardProps) {
  const historyPushedRef = useRef(false);
  const closingFromPopRef = useRef(false);

  const open = useCallback(() => {
    onOpen();
  }, [onOpen]);

  const close = useCallback(() => {
    onClose();
  }, [onClose]);

  useEffect(() => {
    if (!isEditing || typeof window === "undefined") return;

    if (!historyPushedRef.current) {
      window.history.pushState({ ...window.history.state, editableCardId: historyId }, "");
      historyPushedRef.current = true;
    } else {
      window.history.replaceState({ ...window.history.state, editableCardId: historyId }, "");
    }

    const handlePopState = () => {
      if (!historyPushedRef.current) return;
      closingFromPopRef.current = true;
      historyPushedRef.current = false;
      onClose();
    };

    window.addEventListener("popstate", handlePopState);
    return () => {
      window.removeEventListener("popstate", handlePopState);
    };
  }, [historyId, isEditing, onClose]);

  useEffect(() => {
    if (isEditing || typeof window === "undefined") return;
    if (closingFromPopRef.current) {
      closingFromPopRef.current = false;
      historyPushedRef.current = false;
      return;
    }
    if (!historyPushedRef.current) return;
    historyPushedRef.current = false;
    window.history.back();
  }, [isEditing]);

  return isEditing ? renderEdit({ close }) : renderPreview({ open });
}
