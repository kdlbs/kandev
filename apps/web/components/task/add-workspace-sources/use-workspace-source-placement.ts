"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { previewTaskWorkspaceSources } from "@/lib/api/domains/kanban-api";
import type {
  WorkspaceRepositoryPlacement,
  WorkspaceRepositoryPlacementPreview,
} from "@/lib/types/http";
import {
  buildWorkspaceSourcesPayload,
  type WorkspaceSourceRow,
} from "@/components/workspace-source-picker/workspace-source-state";

type Props = {
  open: boolean;
  taskId: string;
  executorType?: string | null;
  rows: WorkspaceSourceRow[];
  errors: Record<string, string>;
};

export function useWorkspaceSourcePlacement({ open, taskId, executorType, rows, errors }: Props) {
  const { t } = useTranslation();
  const [placement, setPlacement] = useState<WorkspaceRepositoryPlacement | null>(null);
  const [preview, setPreview] = useState<WorkspaceRepositoryPlacementPreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const eligible = isWorkspaceRepositoryPlacementEligible(rows, executorType);
  const payload = useMemo(() => {
    const next = buildWorkspaceSourcesPayload(rows);
    if (placement) next.repository_placement = placement;
    return next;
  }, [placement, rows]);
  const previewable = eligible && Object.keys(errors).length === 0;

  useEffect(() => {
    if (!open || !previewable) {
      setPreview(null);
      setPreviewError(null);
      if (!open) setPlacement(null);
      return;
    }
    let cancelled = false;
    setPreview(null);
    setPreviewError(null);
    void previewTaskWorkspaceSources(taskId, payload)
      .then((nextPreview) => {
        if (!cancelled) setPreview(nextPreview);
      })
      .catch((error) => {
        if (!cancelled) {
          setPreview(null);
          setPreviewError(
            error instanceof Error
              ? error.message
              : t("task:workspaceSourcePlacementPreviewFailed"),
          );
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open, payload, previewable, t, taskId]);

  return {
    eligible,
    placement,
    setPlacement,
    preview,
    previewError,
    previewLoading: previewable && !preview && !previewError,
  };
}

function isWorkspaceRepositoryPlacementEligible(
  rows: WorkspaceSourceRow[],
  executorType: string | null | undefined,
): boolean {
  if (rows.length === 0) return false;
  if (executorType === "local" || executorType === "local_pc") {
    return rows.every((row) => {
      if (row.kind === "folder") return Boolean(row.localPath);
      return Boolean(row.repositoryId) && !row.localPath && !row.remoteUrl;
    });
  }
  return (
    executorType === "worktree" &&
    rows.every(
      (row) =>
        row.kind === "repository" && Boolean(row.repositoryId) && !row.localPath && !row.remoteUrl,
    )
  );
}
