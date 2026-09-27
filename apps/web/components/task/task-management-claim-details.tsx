"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type { TFunction } from "i18next";
import {
  changeTaskManagementClaim,
  type TaskManagementClaimSnapshot,
} from "@/lib/api/domains/task-management-claims-api";

type ClaimDetailsProps = {
  snapshot: TaskManagementClaimSnapshot | null;
  error: string | null;
  onRefresh: () => Promise<void>;
};

type ClaimActionsProps = ClaimDetailsProps & {
  taskId: string;
  taskUpdatedAt: string;
  onError: (message: string | null) => void;
};

function ownerLabel(snapshot: TaskManagementClaimSnapshot | null, t: TFunction) {
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

export function TaskManagementClaimActions({
  taskId,
  taskUpdatedAt,
  snapshot,
  onError,
  onRefresh,
}: ClaimActionsProps) {
  const { t } = useTranslation();
  const [reason, setReason] = useState("");
  const [savingAction, setSavingAction] = useState<"transfer" | "release" | null>(null);
  const claim = snapshot?.claim;

  const run = async (action: "transfer" | "release") => {
    if (!claim || !reason.trim() || !taskUpdatedAt || savingAction !== null) return;
    setSavingAction(action);
    onError(null);
    try {
      await changeTaskManagementClaim(taskId, {
        action,
        expected_task_resource_version: taskUpdatedAt,
        expected_claim_resource_version: claim.resource_version,
        reason: reason.trim(),
      });
      setReason("");
    } catch {
      onError(t("task:managementClaimConflict"));
    } finally {
      await onRefresh();
      setSavingAction(null);
    }
  };

  return (
    <div className="flex flex-col gap-3 px-4 pb-4" data-testid="task-management-claim-actions">
      <label
        className="flex flex-col gap-1.5 text-sm font-medium"
        htmlFor="task-management-claim-reason"
      >
        {t("task:managementClaimReason")}
        <Textarea
          id="task-management-claim-reason"
          className="min-h-20 resize-y"
          value={reason}
          onChange={(event) => setReason(event.currentTarget.value)}
          placeholder={t("task:managementClaimReasonPlaceholder")}
          data-testid="task-management-claim-reason"
        />
      </label>
      {claim && (
        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          <Button
            type="button"
            variant="outline"
            disabled={savingAction !== null || !reason.trim()}
            className="min-h-11 cursor-pointer"
            onClick={() => void run("transfer")}
            data-testid="task-management-claim-transfer"
          >
            {savingAction === "transfer"
              ? t("task:managementClaimSaving")
              : t("task:managementClaimTakeOver")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={savingAction !== null || !reason.trim()}
            className="min-h-11 cursor-pointer"
            onClick={() => void run("release")}
            data-testid="task-management-claim-release"
          >
            {savingAction === "release"
              ? t("task:managementClaimSaving")
              : t("task:managementClaimRelease")}
          </Button>
        </div>
      )}
    </div>
  );
}

export function TaskManagementClaimDetails({ snapshot, error, onRefresh }: ClaimDetailsProps) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-4 py-3">
      <div className="rounded-md border border-border p-3">
        <div className="text-xs text-muted-foreground">{t("task:managementClaimCurrentOwner")}</div>
        <div className="break-all text-sm font-medium" data-testid="task-management-claim-owner">
          {ownerLabel(snapshot, t)}
        </div>
      </div>
      {error && (
        <p
          className="text-sm text-destructive"
          role="alert"
          data-testid="task-management-claim-error"
        >
          {error}
        </p>
      )}
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{t("task:managementClaimHistory")}</h3>
        <Button
          type="button"
          variant="ghost"
          className="min-h-11 cursor-pointer"
          onClick={() => void onRefresh()}
          data-testid="task-management-claim-refresh"
        >
          {t("task:managementClaimRefresh")}
        </Button>
      </div>
      <ol className="flex flex-col gap-2" data-testid="task-management-claim-history">
        {(snapshot?.history ?? []).map((item) => (
          <li
            key={item.id}
            className="rounded-md border border-border p-3 text-sm"
            data-testid="task-management-claim-history-item"
          >
            <div className="font-medium">
              {t(`task:managementClaimAction_${item.action}`, { defaultValue: item.action })}
            </div>
            <div className="break-all text-xs text-muted-foreground">{item.actor_id}</div>
            {item.reason && <div className="pt-1 text-xs">{item.reason}</div>}
          </li>
        ))}
        {snapshot && snapshot.history.length === 0 && (
          <li className="text-sm text-muted-foreground">{t("task:managementClaimNoHistory")}</li>
        )}
      </ol>
    </div>
  );
}
