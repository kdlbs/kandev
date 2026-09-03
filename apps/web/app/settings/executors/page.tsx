"use client";

import { useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
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
import { KubernetesReadOnlyNotice } from "@/components/settings/kubernetes-read-only-notice";

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

// `type` is the persisted executor enum and is also the create route's path
// segment — never copy. `brandLabel` is a brand/protocol name rendered
// verbatim; every other label and description travels as a catalog key and
// resolves at render. These sentences are imperative and deliberately differ
// from the third-person ones the create and edit headers show; they are not
// twins of `executors:description*` and must not be folded into them.
type ExecutorTypeCard = {
  type: string;
  labelKey?: string;
  brandLabel?: string;
  descriptionKey: string;
};

const EXECUTOR_TYPES: readonly ExecutorTypeCard[] = [
  {
    type: "local",
    labelKey: "executors:typeLocal",
    descriptionKey: "executors:hubDescriptionLocal",
  },
  {
    type: "worktree",
    labelKey: "executors:typeWorktree",
    descriptionKey: "executors:hubDescriptionWorktree",
  },
  {
    type: "local_docker",
    brandLabel: "Docker",
    descriptionKey: "executors:hubDescriptionDocker",
  },
  {
    type: "sprites",
    brandLabel: "Sprites.dev",
    descriptionKey: "executors:hubDescriptionSprites",
  },
  { type: "ssh", brandLabel: "SSH", descriptionKey: "executors:hubDescriptionSsh" },
  {
    type: "k8s",
    labelKey: "executors:typeKubernetes",
    descriptionKey: "executors:hubDescriptionKubernetes",
  },
];

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
  canDelete,
}: {
  profile: ProfileWithExecutor;
  onDelete: (id: string) => void;
  canDelete: boolean;
}) {
  // Subscribes so the `getExecutorLabel` badge below re-renders on a locale
  // switch; the helper resolves at call time but does not itself notify React.
  const { t } = useTranslation();
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
        </div>
        <Badge variant="outline" className="shrink-0 text-xs">
          {getExecutorLabel(profile.executor_type)}
        </Badge>
        <Button
          variant="ghost"
          size="icon"
          className="min-h-11 w-11 shrink-0 cursor-pointer md:h-8 md:min-h-8 md:w-8 md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100"
          disabled={!canDelete}
          aria-label={t("executors:deleteProfile")}
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
  disabled = false,
}: {
  execType: ExecutorTypeCard;
  onClick: () => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Card
      role="button"
      aria-disabled={disabled}
      tabIndex={disabled ? -1 : 0}
      className={
        disabled
          ? "cursor-not-allowed opacity-60"
          : "cursor-pointer ring-primary/40 transition-colors hover:bg-muted/50"
      }
      onClick={disabled ? undefined : onClick}
      onKeyDown={(event) => {
        if (disabled || (event.key !== "Enter" && event.key !== " ")) return;
        event.preventDefault();
        onClick();
      }}
    >
      <CardContent className="flex items-center gap-3 p-4">
        <ExecutorIconBadge type={execType.type} />
        <div className="min-w-0 flex-1">
          <p className="font-medium">{execType.brandLabel ?? t(execType.labelKey as string)}</p>
          <p className="text-xs text-muted-foreground">{t(execType.descriptionKey)}</p>
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
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("executors:deleteProfile")}</DialogTitle>
          <DialogDescription>
            {t("executors:deleteProfileConfirm", { name: profileName ?? "" })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="min-h-11 cursor-pointer md:min-h-9"
          >
            {t("common:cancel")}
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={deleting}
            className="min-h-11 cursor-pointer md:min-h-9"
          >
            {deleting ? t("executors:deleting") : t("executors:delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function ExecutorsHubPage() {
  const { t } = useTranslation();
  const router = useRouter();
  const allProfiles = useAllProfiles();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const role = useAppStore((state) => state.auth.user?.role);
  const canManageKubernetes = role === undefined || role === "admin";
  const [deleteProfileId, setDeleteProfileId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const profileToDelete = deleteProfileId
    ? allProfiles.find((p) => p.id === deleteProfileId)
    : null;

  const handleDelete = async () => {
    if (!profileToDelete) return;
    if (profileToDelete.executor_type === "k8s" && !canManageKubernetes) return;
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
        <h2 className="text-2xl font-bold">{t("common:executors")}</h2>
        <p className="mt-1 text-sm text-muted-foreground">{t("executors:hubDescription")}</p>
      </div>
      <Separator />
      {!canManageKubernetes && <KubernetesReadOnlyNotice />}
      {allProfiles.length > 0 && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold">{t("executors:profiles")}</h3>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {allProfiles.map((profile) => (
              <ProfileCard
                key={profile.id}
                profile={profile}
                onDelete={setDeleteProfileId}
                canDelete={profile.executor_type !== "k8s" || canManageKubernetes}
              />
            ))}
          </div>
        </div>
      )}
      <div className="space-y-4">
        <h3 className="text-lg font-semibold">{t("executors:createNewProfile")}</h3>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {EXECUTOR_TYPES.map((execType) => (
            <CreateTypeCard
              key={execType.type}
              execType={execType}
              onClick={() => router.push(`/settings/executors/new/${execType.type}`)}
              disabled={execType.type === "k8s" && !canManageKubernetes}
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
