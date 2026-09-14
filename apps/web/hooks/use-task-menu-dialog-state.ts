"use client";

import { useCallback, useState } from "react";
import type { ExternalLinkProvider } from "@/components/task/task-external-link-dialog";

/** Every confirm/link-dialog open flag a task actions menu and its dialogs share.
 * `closeAll` resets every flag; callers use it whenever the subject a dialog
 * was opened for is no longer the current one (AC-TASKS-TASK-ACTIONS-MENU-004.5a's
 * no-retarget rule applies to these dialogs, not just the trigger itself). */
export function useTaskMenuDialogState() {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const [showDetachConfirm, setShowDetachConfirm] = useState(false);
  const [showPRDialog, setShowPRDialog] = useState(false);
  const [showIssueDialog, setShowIssueDialog] = useState(false);
  const [showMRDialog, setShowMRDialog] = useState(false);
  const [externalLinkProvider, setExternalLinkProvider] = useState<ExternalLinkProvider | null>(
    null,
  );
  const closeAll = useCallback(() => {
    setShowDeleteConfirm(false);
    setShowArchiveConfirm(false);
    setShowDetachConfirm(false);
    setShowPRDialog(false);
    setShowIssueDialog(false);
    setShowMRDialog(false);
    setExternalLinkProvider(null);
  }, []);
  return {
    showDeleteConfirm,
    setShowDeleteConfirm,
    showArchiveConfirm,
    setShowArchiveConfirm,
    showDetachConfirm,
    setShowDetachConfirm,
    showPRDialog,
    setShowPRDialog,
    showIssueDialog,
    setShowIssueDialog,
    showMRDialog,
    setShowMRDialog,
    externalLinkProvider,
    setExternalLinkProvider,
    closeAll,
  };
}

/** Edit-dialog open flag for a single-subject task actions menu (preview/detail). */
export function useTaskMenuEditDialogState() {
  const [showEditDialog, setShowEditDialog] = useState(false);
  const closeAll = useCallback(() => setShowEditDialog(false), []);
  return { showEditDialog, setShowEditDialog, closeAll };
}
