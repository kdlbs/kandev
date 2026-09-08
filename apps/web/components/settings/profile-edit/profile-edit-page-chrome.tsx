"use client";

import type { ReactNode } from "react";
import { useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
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
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { SettingsPageHeader, SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";

const EXECUTORS_ROUTE = "/settings/executors";
const DefaultIcon = EXECUTOR_ICON_MAP.local;
export type SaveStatus = "idle" | "loading" | "success" | "error";

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
  actions,
}: {
  executor: Executor;
  profileName: string;
  description: string;
  actions?: ReactNode;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <>
      <SettingsPageHeader
        title={
          <span className="flex min-w-0 flex-wrap items-center gap-2 break-words">
            <ExecutorTypeIcon type={executor.type} />
            <span className="min-w-0 break-words">{profileName}</span>
            <Badge variant="outline" className={SETTINGS_TYPOGRAPHY.meta}>
              {getExecutorLabel(executor.type)}
            </Badge>
          </span>
        }
        description={description}
        actions={
          <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row md:items-center">
            {actions}
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push(EXECUTORS_ROUTE)}
              className="min-h-11 w-full cursor-pointer text-sm md:min-h-7 md:w-auto md:text-xs"
            >
              {t("executors:backToExecutors")}
            </Button>
          </div>
        }
      />
      <Separator />
    </>
  );
}

export function ProfileFormActions({
  onDelete,
  disabled = false,
}: {
  onDelete: () => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
      <Button
        variant="destructive"
        size="sm"
        onClick={onDelete}
        disabled={disabled}
        className="min-h-11 cursor-pointer text-sm md:min-h-7 md:text-xs"
      >
        <IconTrash className="mr-1 h-4 w-4" />
        {t("executors:deleteProfile")}
      </Button>
      <Button
        variant="outline"
        onClick={() => router.push(EXECUTORS_ROUTE)}
        className="min-h-11 cursor-pointer text-sm md:min-h-7 md:text-xs"
      >
        {t("common:cancel")}
      </Button>
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
  const { t } = useTranslation();
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
          <DialogTitle>{t("executors:deleteProfile")}</DialogTitle>
          <DialogDescription>{t("executors:areYouSureThisActionCannot")}</DialogDescription>
        </DialogHeader>
        {hasRelatedContainers && (
          <div className="space-y-3 rounded-md border p-3">
            <p className="text-sm text-muted-foreground">
              {t("executors:relatedDockerContainersRemoved", {
                count: relatedDockerContainerCount,
              })}
            </p>
            <div className="flex items-center gap-2">
              <Checkbox
                id="remove-related-docker-containers"
                checked={shouldRemoveRelatedContainers}
                onCheckedChange={(checked) => setRemoveRelatedContainers(checked === true)}
              />
              <Label htmlFor="remove-related-docker-containers" className="cursor-pointer text-sm">
                {t("executors:removeRelatedDockerContainers")}
              </Label>
            </div>
          </div>
        )}
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
            onClick={() =>
              onDelete({ removeRelatedDockerContainers: shouldRemoveRelatedContainers })
            }
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
