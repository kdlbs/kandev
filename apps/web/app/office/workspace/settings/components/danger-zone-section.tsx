"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { IconTrash } from "@tabler/icons-react";
import { toast } from "sonner";
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

type Workspace = WorkspaceState["items"][number];

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
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="workspace-delete-dialog">
        <DialogHeader>
          <DialogTitle>Delete workspace</DialogTitle>
          <DialogDescription>
            This will permanently delete {summary?.agents ?? 0} agents, {summary?.tasks ?? 0} tasks,{" "}
            {summary?.skills ?? 0} skills, and the workspace folder.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="rounded-md border bg-muted/30 px-3 py-2 font-mono text-xs break-all">
            {summary?.config_path}
          </div>
          <div className="space-y-2">
            <Label htmlFor="delete-workspace-confirm">Type {confirmName} to confirm</Label>
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
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={!canDelete}
            className="cursor-pointer"
            data-testid="workspace-delete-confirm-button"
          >
            {deleting ? "Deleting..." : "Delete"}
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
      toast.error(err instanceof Error ? err.message : "Failed to load deletion summary");
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
      const nextWorkspace = remaining[0] ?? null;
      setWorkspaces(remaining);
      setActiveWorkspace(nextWorkspace?.id ?? null);
      router.push(nextWorkspace ? "/office" : "/office/setup");
      toast.success("Workspace deleted");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete workspace");
    } finally {
      setDeleting(false);
    }
  };

  return (
    <SettingCard>
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-destructive">Delete workspace</p>
          <p className="text-xs text-muted-foreground mt-1">
            This permanently deletes agents, tasks, skills, routines, and configuration.
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
          {loading ? "Loading..." : "Delete workspace"}
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
