"use client";

import { useCallback, useState } from "react";
import type { RemoveSessionOptions } from "@/hooks/domains/session/use-session-actions";

type DeleteOrigin = "tab" | "menu" | null;
type SetConfirmDelete = (open: boolean) => void;
type HandleDelete = (options?: RemoveSessionOptions) => Promise<boolean>;

export function useSessionTabDelete(
  setConfirmDelete: SetConfirmDelete,
  handleDelete: HandleDelete,
) {
  const [deleteOrigin, setDeleteOrigin] = useState<DeleteOrigin>(null);
  const [isDeletingFromTab, setIsDeletingFromTab] = useState(false);

  const handleCloseTab = useCallback(() => {
    setDeleteOrigin("tab");
    setConfirmDelete(true);
  }, [setConfirmDelete]);

  const handleDeleteDialogOpenChange = useCallback(
    (open: boolean) => {
      setConfirmDelete(open);
      if (!open) setDeleteOrigin(null);
    },
    [setConfirmDelete],
  );

  const handleConfirmDelete = useCallback(async () => {
    const isTabDelete = deleteOrigin === "tab";
    if (isTabDelete) setIsDeletingFromTab(true);

    try {
      await handleDelete({ feedback: isTabDelete ? "inline" : "toast" });
    } finally {
      if (isTabDelete) setIsDeletingFromTab(false);
      setDeleteOrigin(null);
    }
  }, [deleteOrigin, handleDelete]);

  const handleMenuDelete = useCallback(() => {
    setDeleteOrigin("menu");
    setConfirmDelete(true);
  }, [setConfirmDelete]);

  return {
    deleteOrigin,
    handleCloseTab,
    handleDeleteDialogOpenChange,
    handleConfirmDelete,
    handleMenuDelete,
    isDeletingFromTab,
  };
}
