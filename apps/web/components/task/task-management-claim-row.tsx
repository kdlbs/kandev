"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  getTaskManagementClaim,
  type TaskManagementClaimSnapshot,
} from "@/lib/api/domains/task-management-claims-api";
import { TaskManagementClaimSurface } from "./task-management-claim-surface";

type TaskManagementClaimRowProps = {
  taskId: string;
  taskUpdatedAt: string;
  isMobile: boolean;
};

function ownerText(
  snapshot: TaskManagementClaimSnapshot | null,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const claim = snapshot?.claim;
  if (!claim || !claim.owner_kind) return t("task:managementClaimNone");
  if (claim.owner_kind === "human") {
    return t("task:managementClaimHumanOwner", { actorId: claim.owner_actor_id ?? "" });
  }
  return t("task:managementClaimPluginOwner", {
    installationId: claim.installation_id ?? "",
    instanceKey: claim.instance_key ?? "",
  });
}

export function TaskManagementClaimRow({
  taskId,
  taskUpdatedAt,
  isMobile,
}: TaskManagementClaimRowProps) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState<TaskManagementClaimSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    try {
      setSnapshot(await getTaskManagementClaim(taskId));
    } catch {
      setError(t("task:managementClaimLoadError"));
    } finally {
      setLoading(false);
    }
  }, [taskId, t]);

  useEffect(() => {
    setSnapshot(null);
    void load();
  }, [load]);

  return (
    <section
      className={`flex shrink-0 items-center justify-between gap-3 border-b border-border px-4 py-2 ${isMobile ? "mt-14" : ""}`}
      data-testid="task-management-claim-row"
    >
      <div className="flex min-w-0 flex-col">
        <span className="text-xs text-muted-foreground">{t("task:managementClaimManager")}</span>
        <span className="truncate text-sm font-medium" data-testid="task-management-claim-owner">
          {loading ? t("task:managementClaimLoading") : ownerText(snapshot, t)}
        </span>
        {error && (
          <span className="text-xs text-destructive" role="alert">
            {error}
          </span>
        )}
      </div>
      <Button
        type="button"
        variant="outline"
        className="min-h-11 shrink-0 cursor-pointer"
        onClick={() => setOpen(true)}
        data-testid="task-management-claim-open"
      >
        {t("task:managementClaimManage")}
      </Button>
      <TaskManagementClaimSurface
        taskId={taskId}
        taskUpdatedAt={taskUpdatedAt}
        isMobile={isMobile}
        open={open}
        snapshot={snapshot}
        error={error}
        onOpenChange={setOpen}
        onRefresh={load}
        onError={setError}
      />
    </section>
  );
}
