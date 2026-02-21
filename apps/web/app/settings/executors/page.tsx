"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { useAppStore } from "@/components/state-provider";
import { deleteExecutorProfile } from "@/lib/api/domains/settings-api";
import { EXECUTOR_ICON_MAP, getExecutorLabel } from "@/lib/executor-icons";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

type ProfileWithExecutor = ExecutorProfile & {
  executor_type: string;
  executor_name: string;
  parent_executor_id: string;
};

function useAllProfiles(): ProfileWithExecutor[] {
  const executors = useAppStore((state) => state.executors.items);
  return useMemo(
    () =>
      executors.flatMap((e: Executor) =>
        (e.profiles ?? []).map((p) => ({
          ...p,
          executor_type: e.type,
          executor_name: e.name,
          parent_executor_id: e.id,
        })),
      ),
    [executors],
  );
}

const DefaultIcon = EXECUTOR_ICON_MAP.local;

const EXECUTOR_TYPES = [
  { type: "local", label: "Local", description: "Run agents directly in the repository folder." },
  {
    type: "worktree",
    label: "Worktree",
    description: "Create git worktrees for isolated agent sessions.",
  },
  { type: "local_docker", label: "Docker", description: "Run Docker containers on this machine." },
  {
    type: "sprites",
    label: "Sprites.dev",
    description: "Run agents in Sprites.dev cloud sandboxes.",
  },
] as const;

function ExecutorIconBadge({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return (
    <div className="rounded-md bg-muted p-2">
      <Icon className="h-4 w-4" />
    </div>
  );
}

function ProfileCard({
  profile,
  onDelete,
}: {
  profile: ProfileWithExecutor;
  onDelete: (id: string) => void;
}) {
  const router = useRouter();
  return (
    <Card
      className="group cursor-pointer transition-colors hover:bg-muted/50"
      onClick={() => router.push(`/settings/executors/${profile.id}`)}
    >
      <CardContent className="flex items-center gap-3 p-4">
        <ExecutorIconBadge type={profile.executor_type} />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{profile.name}</p>
          <p className="text-xs text-muted-foreground">{getExecutorLabel(profile.executor_type)}</p>
        </div>
        <Badge variant="outline" className="shrink-0 text-xs">
          {getExecutorLabel(profile.executor_type)}
        </Badge>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 shrink-0 cursor-pointer opacity-0 group-hover:opacity-100"
          onClick={(e) => {
            e.stopPropagation();
            onDelete(profile.id);
          }}
        >
          <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </CardContent>
    </Card>
  );
}

function CreateTypeCard({
  execType,
  onClick,
}: {
  execType: (typeof EXECUTOR_TYPES)[number];
  onClick: () => void;
}) {
  return (
    <Card className="cursor-pointer transition-colors hover:bg-muted/50" onClick={onClick}>
      <CardContent className="flex items-center gap-3 p-4">
        <ExecutorIconBadge type={execType.type} />
        <div className="min-w-0 flex-1">
          <p className="font-medium">{execType.label}</p>
          <p className="text-xs text-muted-foreground">{execType.description}</p>
        </div>
        <IconPlus className="h-4 w-4 shrink-0 text-muted-foreground" />
      </CardContent>
    </Card>
  );
}

function DeleteProfileDialog({
  profileName,
  open,
  onOpenChange,
  onDelete,
  deleting,
}: {
  profileName: string | undefined;
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
          <DialogDescription>
            Are you sure you want to delete &quot;{profileName}&quot;? This action cannot be undone.
          </DialogDescription>
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

export default function ExecutorsHubPage() {
  const router = useRouter();
  const allProfiles = useAllProfiles();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [deleteProfileId, setDeleteProfileId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const profileToDelete = deleteProfileId
    ? allProfiles.find((p) => p.id === deleteProfileId)
    : null;

  const handleDelete = async () => {
    if (!profileToDelete) return;
    setDeleting(true);
    try {
      await deleteExecutorProfile(profileToDelete.parent_executor_id, profileToDelete.id);
      setExecutors(
        executors.map((e: Executor) =>
          e.id === profileToDelete.parent_executor_id
            ? { ...e, profiles: (e.profiles ?? []).filter((p) => p.id !== profileToDelete.id) }
            : e,
        ),
      );
      setDeleteProfileId(null);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Executors</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Executor profiles define how and where agents run. Each profile configures an execution
          environment with scripts, environment variables, and MCP policies.
        </p>
      </div>
      <Separator />
      {allProfiles.length > 0 && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold">Profiles</h3>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {allProfiles.map((profile) => (
              <ProfileCard key={profile.id} profile={profile} onDelete={setDeleteProfileId} />
            ))}
          </div>
        </div>
      )}
      <div className="space-y-4">
        <h3 className="text-lg font-semibold">Create New Profile</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          {EXECUTOR_TYPES.map((execType) => (
            <CreateTypeCard
              key={execType.type}
              execType={execType}
              onClick={() => router.push(`/settings/executors/new/${execType.type}`)}
            />
          ))}
        </div>
      </div>
      <DeleteProfileDialog
        profileName={profileToDelete?.name}
        open={Boolean(deleteProfileId)}
        onOpenChange={() => setDeleteProfileId(null)}
        onDelete={handleDelete}
        deleting={deleting}
      />
    </div>
  );
}
