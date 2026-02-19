"use client";

import { use, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { Textarea } from "@kandev/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { updateEnvironmentAction, deleteEnvironmentAction } from "@/app/actions/environments";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Environment } from "@/lib/types/http";
import type { BaseDocker } from "@/lib/settings/types";
import { useAppStore } from "@/components/state-provider";

const ENVIRONMENTS_ROUTE = "/settings/environments";

const BASE_IMAGE_LABELS: Record<BaseDocker, string> = {
  universal: "Universal (Ubuntu)",
  golang: "Golang",
  node: "Node.js",
  python: "Python",
};

export default function EnvironmentEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const environment = useAppStore(
    (state) => state.environments.items.find((item: Environment) => item.id === id) ?? null,
  );

  if (!environment) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Environment not found</p>
            <Button className="mt-4" onClick={() => router.push(ENVIRONMENTS_ROUTE)}>
              Go to Environments
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <EnvironmentEditForm key={environment.id} environment={environment} />;
}

type EnvironmentEditFormProps = {
  environment: Environment;
};

type DeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
  isDeleting: boolean;
  confirmText: string;
  onConfirmTextChange: (value: string) => void;
};

function DeleteEnvironmentDialog({
  open,
  onOpenChange,
  onDelete,
  isDeleting,
  confirmText,
  onConfirmTextChange,
}: DeleteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Environment</DialogTitle>
          <DialogDescription>
            Type &quot;delete&quot; to confirm deletion. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="confirm-delete">Confirm Delete</Label>
          <Input
            id="confirm-delete"
            value={confirmText}
            onChange={(event) => onConfirmTextChange(event.target.value)}
            placeholder="delete"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={confirmText !== "delete" || isDeleting}
          >
            {isDeleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EnvironmentDetailsCard({
  draft,
  isSystem,
  onDraftChange,
}: {
  draft: Environment;
  isSystem: boolean;
  onDraftChange: (next: Environment) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Environment Details</CardTitle>
        <CardDescription>
          {isSystem
            ? "System environments only allow worktree configuration."
            : "Customize the base image and worktree settings."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="environment-name">Environment name</Label>
          <Input
            id="environment-name"
            value={draft.name}
            onChange={(event) => onDraftChange({ ...draft, name: event.target.value })}
            disabled={isSystem}
          />
          {isSystem && (
            <p className="text-xs text-muted-foreground">
              System environment names cannot be edited.
            </p>
          )}
        </div>
        {!isSystem && (
          <div className="space-y-2">
            <Label htmlFor="environment-kind">Base image</Label>
            <Select
              value={draft.kind}
              onValueChange={(value) => onDraftChange({ ...draft, kind: value as BaseDocker })}
            >
              <SelectTrigger id="environment-kind">
                <SelectValue placeholder="Select base image" />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(BASE_IMAGE_LABELS).map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="worktree-root">Worktree root</Label>
          <Input
            id="worktree-root"
            value={draft.worktree_root ?? ""}
            onChange={(event) =>
              onDraftChange({ ...draft, worktree_root: event.target.value || null })
            }
            placeholder="/workspace"
          />
        </div>
      </CardContent>
    </Card>
  );
}

function DockerfileCard({
  dockerfile,
  onChange,
}: {
  dockerfile: string;
  onChange: (value: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Custom Dockerfile</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <Label htmlFor="dockerfile">Dockerfile override</Label>
        <Textarea
          id="dockerfile"
          value={dockerfile}
          onChange={(event) => onChange(event.target.value)}
          placeholder="# Optional custom Dockerfile"
          rows={6}
        />
      </CardContent>
    </Card>
  );
}

function DeleteEnvironmentSection({
  onDeleteClick,
  deleteDialogOpen,
  setDeleteDialogOpen,
  onDelete,
  isDeleting,
  confirmText,
  onConfirmTextChange,
}: {
  onDeleteClick: () => void;
  deleteDialogOpen: boolean;
  setDeleteDialogOpen: (open: boolean) => void;
  onDelete: () => void;
  isDeleting: boolean;
  confirmText: string;
  onConfirmTextChange: (value: string) => void;
}) {
  return (
    <>
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Delete Environment</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">Remove this environment</p>
            <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
          </div>
          <Button variant="destructive" onClick={onDeleteClick}>
            <IconTrash className="h-4 w-4 mr-2" />
            Delete
          </Button>
        </CardContent>
      </Card>
      <DeleteEnvironmentDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        onDelete={onDelete}
        isDeleting={isDeleting}
        confirmText={confirmText}
        onConfirmTextChange={onConfirmTextChange}
      />
    </>
  );
}

function buildEnvironmentPayload(draft: Environment, isSystem: boolean): Record<string, unknown> {
  const payload: Record<string, unknown> = {
    worktree_root: draft.worktree_root ?? undefined,
  };
  if (!isSystem) {
    payload.name = draft.name;
    payload.kind = draft.kind;
    payload.image_tag = draft.image_tag ?? undefined;
    payload.dockerfile = draft.dockerfile ?? undefined;
    payload.build_config = draft.build_config ?? undefined;
  }
  return payload;
}

function EnvironmentEditForm({ environment }: EnvironmentEditFormProps) {
  const router = useRouter();
  const [draft, setDraft] = useState<Environment>({ ...environment });
  const [savedEnvironment, setSavedEnvironment] = useState<Environment>({ ...environment });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const isSystem = draft.is_system ?? false;

  const isDirty = useMemo(() => {
    if (isSystem) return draft.worktree_root !== savedEnvironment.worktree_root;
    return (
      draft.name !== savedEnvironment.name ||
      draft.kind !== savedEnvironment.kind ||
      draft.worktree_root !== savedEnvironment.worktree_root ||
      draft.image_tag !== savedEnvironment.image_tag ||
      draft.dockerfile !== savedEnvironment.dockerfile ||
      JSON.stringify(draft.build_config ?? {}) !==
        JSON.stringify(savedEnvironment.build_config ?? {})
    );
  }, [draft, savedEnvironment, isSystem]);

  const handleDeleteEnvironment = async () => {
    if (deleteConfirmText !== "delete") return;
    setIsDeleting(true);
    try {
      const client = getWebSocketClient();
      if (client) {
        await client.request("environment.delete", { id: draft.id });
      } else {
        await deleteEnvironmentAction(draft.id);
      }
      router.push(ENVIRONMENTS_ROUTE);
    } finally {
      setIsDeleting(false);
      setDeleteDialogOpen(false);
    }
  };

  const handleSaveEnvironment = async () => {
    setIsSaving(true);
    try {
      const payload = buildEnvironmentPayload(draft, isSystem);
      const client = getWebSocketClient();
      const updated = client
        ? await client.request<Environment>("environment.update", { id: draft.id, ...payload })
        : await updateEnvironmentAction(draft.id, payload);
      setDraft(updated);
      setSavedEnvironment(updated);
      router.push(ENVIRONMENTS_ROUTE);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <h2 className="text-2xl font-bold">{draft.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Configure runtime environment settings.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => router.push(ENVIRONMENTS_ROUTE)}>
          Back to Environments
        </Button>
      </div>
      <Separator />
      <EnvironmentDetailsCard draft={draft} isSystem={isSystem} onDraftChange={setDraft} />
      {!isSystem && (
        <DockerfileCard
          dockerfile={draft.dockerfile ?? ""}
          onChange={(value) => setDraft({ ...draft, dockerfile: value })}
        />
      )}
      <Separator />
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" onClick={() => router.push(ENVIRONMENTS_ROUTE)}>
          Cancel
        </Button>
        <Button onClick={handleSaveEnvironment} disabled={!isDirty || isSaving}>
          {isSaving ? "Saving..." : "Save Changes"}
        </Button>
      </div>
      {!isSystem && (
        <DeleteEnvironmentSection
          onDeleteClick={() => setDeleteDialogOpen(true)}
          deleteDialogOpen={deleteDialogOpen}
          setDeleteDialogOpen={setDeleteDialogOpen}
          onDelete={handleDeleteEnvironment}
          isDeleting={isDeleting}
          confirmText={deleteConfirmText}
          onConfirmTextChange={setDeleteConfirmText}
        />
      )}
    </div>
  );
}
