"use client";

import { useState } from "react";
import type { ExternalLinkProvider } from "@/components/task/task-external-link-dialog";

/** Every confirm/link-dialog open flag a task actions menu and its dialogs share. */
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
  };
}

/** Edit-dialog open flag for a single-subject task actions menu (preview/detail). */
export function useTaskMenuEditDialogState() {
  const [showEditDialog, setShowEditDialog] = useState(false);
  return { showEditDialog, setShowEditDialog };
}
