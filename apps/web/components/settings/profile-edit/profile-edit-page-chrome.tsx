"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
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
import { RequestIndicator } from "@/components/request-indicator";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";
const DefaultIcon = EXECUTOR_ICON_MAP.local;
export type SaveStatus = "idle" | "loading" | "success" | "error";

export function getSaveButtonLabel(status: SaveStatus) {
  if (status === "success") return "Saved";
  if (status === "loading") return "Saving";
  return "Save Changes";
}

export function upsertExecutorProfile(
  executors: Executor[],
  executor: Executor,
  updated: ExecutorProfile,
) {
  let foundExecutor = false;
  const replaceProfile = (profiles: ExecutorProfile[] = []) => {
    const foundProfile = profiles.some((p) => p.id === updated.id);
    if (!foundProfile) return [...profiles, updated];
    return profiles.map((p) => (p.id === updated.id ? updated : p));
  };

  const next = executors.map((item) => {
    if (item.id !== executor.id) return item;
    foundExecutor = true;
    return { ...item, profiles: replaceProfile(item.profiles ?? executor.profiles ?? []) };
  });

  if (foundExecutor) return next;
  return [...next, { ...executor, profiles: replaceProfile(executor.profiles ?? []) }];
}

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
  saveStatus,
  saveDisabled,
  onSave,
  onDelete,
}: {
  saveStatus: SaveStatus;
  saveDisabled: boolean;
  onSave: () => void;
  onDelete: () => void;
}) {
  const router = useRouter();
  const saveLabel = getSaveButtonLabel(saveStatus);
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
        <Button onClick={onSave} disabled={saveDisabled} className="min-w-36 cursor-pointer">
          {saveLabel}
          {saveStatus !== "idle" && (
            <span className="ml-2">
              <RequestIndicator status={saveStatus} />
            </span>
          )}
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
  relatedDockerContainerCount = 0,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDelete: (options?: { removeRelatedDockerContainers?: boolean }) => void;
  deleting: boolean;
  relatedDockerContainerCount?: number;
}) {
  const [removeRelatedContainers, setRemoveRelatedContainers] = useState<boolean | null>(null);
  const hasRelatedContainers = relatedDockerContainerCount > 0;
  const shouldRemoveRelatedContainers = hasRelatedContainers && (removeRelatedContainers ?? true);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) setRemoveRelatedContainers(null);
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Profile</DialogTitle>
          <DialogDescription>Are you sure? This action cannot be undone.</DialogDescription>
        </DialogHeader>
        {hasRelatedContainers && (
          <div className="space-y-3 rounded-md border p-3">
            <p className="text-sm text-muted-foreground">
              {relatedDockerContainerCount} related Docker{" "}
              {relatedDockerContainerCount === 1 ? "container" : "containers"} will also be removed.
            </p>
            <div className="flex items-center gap-2">
              <Checkbox
                id="remove-related-docker-containers"
                checked={shouldRemoveRelatedContainers}
                onCheckedChange={(checked) => setRemoveRelatedContainers(checked === true)}
              />
              <Label htmlFor="remove-related-docker-containers" className="cursor-pointer text-sm">
                Remove related Docker containers
              </Label>
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() =>
              onDelete({ removeRelatedDockerContainers: shouldRemoveRelatedContainers })
            }
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
