"use client";

import { useRouter } from "next/navigation";
import { IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { EXECUTOR_ICON_MAP, getExecutorLabel } from "@/lib/executor-icons";
import type { Executor } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";
const DefaultIcon = EXECUTOR_ICON_MAP.local;

function ExecutorTypeIcon({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return <Icon className="h-5 w-5 text-muted-foreground" />;
}

export function ProfileHeader({
  executor,
  profileName,
  description,
}: {
  executor: Executor;
  profileName: string;
  description: string;
}) {
  const router = useRouter();
  return (
    <>
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <ExecutorTypeIcon type={executor.type} />
            <h2 className="text-2xl font-bold">{profileName}</h2>
            <Badge variant="outline" className="text-xs">
              {getExecutorLabel(executor.type)}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="cursor-pointer"
        >
          Back to Executors
        </Button>
      </div>
      <Separator />
    </>
  );
}

export function ProfileFormActions({
  saving,
  saveDisabled,
  onSave,
  onDelete,
}: {
  saving: boolean;
  saveDisabled: boolean;
  onSave: () => void;
  onDelete: () => void;
}) {
  const router = useRouter();
  return (
    <div className="flex items-center justify-between">
      <Button variant="destructive" size="sm" onClick={onDelete} className="cursor-pointer">
        <IconTrash className="mr-1 h-4 w-4" />
        Delete Profile
      </Button>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="cursor-pointer"
        >
          Cancel
        </Button>
        <Button onClick={onSave} disabled={saveDisabled} className="cursor-pointer">
          {saving ? "Saving..." : "Save Changes"}
        </Button>
      </div>
    </div>
  );
}

export function DeleteProfileDialog({
  open,
  onOpenChange,
  onDelete,
  deleting,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDelete: () => void;
  deleting: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Profile</DialogTitle>
          <DialogDescription>Are you sure? This action cannot be undone.</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={deleting}
            className="cursor-pointer"
          >
            {deleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
