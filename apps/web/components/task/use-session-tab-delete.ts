"use client";

import { useCallback, useState } from "react";
import type { RemoveSessionOptions } from "@/hooks/domains/session/use-session-actions";

type DeleteOrigin = "menu" | null;
type SetConfirmDelete = (open: boolean) => void;
type HandleDelete = (options?: RemoveSessionOptions) => Promise<boolean>;

export function useSessionTabDelete(
  setConfirmDelete: SetConfirmDelete,
  handleDelete: HandleDelete,
) {
  const [deleteOrigin, setDeleteOrigin] = useState<DeleteOrigin>(null);
  const handleDeleteDialogOpenChange = useCallback(
    (open: boolean) => {
      setConfirmDelete(open);
      if (!open) setDeleteOrigin(null);
    },
    [setConfirmDelete],
  );

  const handleConfirmDelete = useCallback(async () => {
    try {
      await handleDelete({ feedback: "toast" });
    } finally {
      setDeleteOrigin(null);
    }
  }, [deleteOrigin, handleDelete]);

  const handleMenuDelete = useCallback(() => {
    setDeleteOrigin("menu");
    setConfirmDelete(true);
  }, [setConfirmDelete]);

  return {
    deleteOrigin,
    handleDeleteDialogOpenChange,
    handleConfirmDelete,
    handleMenuDelete,
  };
}
