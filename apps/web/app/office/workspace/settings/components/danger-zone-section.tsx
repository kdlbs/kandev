"use client";

import { useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconTrash } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  deleteWorkspace,
  getWorkspaceDeletionSummary,
  type WorkspaceDeletionSummary,
} from "@/lib/api/domains/office-api";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import { isOfficeWorkspace } from "@/lib/state/slices/workspace/selectors";
import { workspaceHomeHref } from "@/lib/navigation/workspace-home";
import { useTranslation } from "react-i18next";

type Workspace = WorkspaceState["items"][number];

export function resolvePostDeleteWorkspace(
  deletedWorkspaceId: string,
  workspaces: Workspace[],
): Workspace | null {
  const remaining = workspaces.filter((item) => item.id !== deletedWorkspaceId);
  return remaining.find(isOfficeWorkspace) ?? remaining[0] ?? null;
}

export function postDeleteWorkspaceHref(workspace: Workspace | null): string {
  return workspace ? workspaceHomeHref(workspace) : "/office/setup?mode=new";
}

function SettingCard({ children }: { children: React.ReactNode }) {
  return <div className="rounded-lg border border-border p-4 space-y-4">{children}</div>;
}

function DeleteWorkspaceDialog({
  open,
  onOpenChange,
  summary,
  confirmName,
  confirmText,
  deleting,
  canDelete,
  onConfirmTextChange,
  onDelete,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  summary: WorkspaceDeletionSummary | null;
  confirmName: string;
  confirmText: string;
  deleting: boolean;
  canDelete: boolean;
  onConfirmTextChange: (value: string) => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="workspace-delete-dialog">
        <DialogHeader>
          <DialogTitle>{t("office:deleteWorkspace")}</DialogTitle>
          <DialogDescription>
            {t("office:deleteWorkspaceSummary", {
              agents: summary?.agents ?? 0,
              tasks: summary?.tasks ?? 0,
              skills: summary?.skills ?? 0,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="rounded-md border bg-muted/30 px-3 py-2 font-mono text-xs break-all">
            {summary?.config_path}
          </div>
          <div className="space-y-2">
            {/*
              `confirmName` is the workspace's own name — user data, interpolated
              and never translated, so `confirmText === confirmName` above stays
              typeable in every locale. One key, so the instruction can put the
              name wherever the language needs it.
            */}
            <Label htmlFor="delete-workspace-confirm">
              {t("office:typeNameToConfirm", { name: confirmName })}
            </Label>
            <Input
              id="delete-workspace-confirm"
              data-testid="workspace-delete-confirm-input"
              value={confirmText}
              onChange={(event) => onConfirmTextChange(event.target.value)}
              autoComplete="off"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={!canDelete}
            className="cursor-pointer"
            data-testid="workspace-delete-confirm-button"
          >
            {deleting ? t("office:deleting") : t("office:delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function DangerZoneSection({
  workspace,
  workspaces,
  setWorkspaces,
  setActiveWorkspace,
}: {
  workspace: Workspace;
  workspaces: Workspace[];
  setWorkspaces: (items: Workspace[]) => void;
  setActiveWorkspace: (id: string | null) => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");
  const [summary, setSummary] = useState<WorkspaceDeletionSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const confirmName = summary?.workspace_name ?? workspace.name ?? "";
  const canDelete = confirmText === confirmName && !deleting;

  const openDialog = async () => {
    setLoading(true);
    try {
      const nextSummary = await getWorkspaceDeletionSummary(workspace.id);
      setSummary(nextSummary);
      setConfirmText("");
      setOpen(true);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToLoadDeletionSummary"));
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!canDelete) return;
    setDeleting(true);
    try {
      await deleteWorkspace(workspace.id, confirmName);
      const remaining = workspaces.filter((item) => item.id !== workspace.id);
      const nextWorkspace = resolvePostDeleteWorkspace(workspace.id, workspaces);
      setWorkspaces(remaining);
      setActiveWorkspace(nextWorkspace?.id ?? null);
      router.push(postDeleteWorkspaceHref(nextWorkspace));
      toast.success(t("office:workspaceDeleted"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToDeleteWorkspace"));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <SettingCard>
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-destructive">{t("office:deleteWorkspace")}</p>
          <p className="text-xs text-muted-foreground mt-1">
            {t("office:thisPermanentlyDeletesAgentsTasksSkills")}
          </p>
        </div>
        <Button
          variant="destructive"
          size="sm"
          onClick={openDialog}
          disabled={loading}
          className="cursor-pointer shrink-0"
          data-testid="workspace-delete-button"
        >
          <IconTrash className="h-4 w-4 mr-1.5" />
          {loading ? t("common:loadingEllipsis") : t("office:deleteWorkspace")}
        </Button>
      </div>
      <DeleteWorkspaceDialog
        open={open}
        onOpenChange={setOpen}
        summary={summary}
        confirmName={confirmName}
        confirmText={confirmText}
        deleting={deleting}
        canDelete={canDelete}
        onConfirmTextChange={setConfirmText}
        onDelete={handleDelete}
      />
    </SettingCard>
  );
}
