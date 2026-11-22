"use client";
import { useTranslation } from "react-i18next";
import type { SSHManager, SSHOperation } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import { AccessStatus } from "./access-primitives";

export function currentOperation(manager: SSHManager, route: SSHRoute): SSHOperation | null {
  const operation = manager.operation;
  return operation?.connectionId === route.destination.id && operation.stamp === route.stamp
    ? operation
    : null;
}
export function OperationFeedback({
  manager,
  route,
  kind,
}: {
  manager: SSHManager;
  route: SSHRoute;
  kind?: SSHOperation["kind"];
}) {
  const { t } = useTranslation();
  const operation = currentOperation(manager, route);
  if (!operation || (kind && operation.kind !== kind)) return null;
  if (operation.status === "succeeded") {
    if (operation.kind !== "test") return null;
    return <AccessStatus tone="success" title={t("settings:sshManager.ready")} />;
  }
  if (operation.status === "pending")
    return (
      <AccessStatus pending title={t(`settings:sshManager.experience.pending.${operation.kind}`)} />
    );
  return (
    <AccessStatus tone="error" title={t(`settings:sshManager.experience.failed.${operation.kind}`)}>
      {operation.hop && (
        <span className="font-medium">{t(`settings:sshManager.${operation.hop}`)}: </span>
      )}
      {t(`settings:sshManager.errors.${operation.error}`, {
        defaultValue: t("settings:sshManager.errors.ssh_operation_failed"),
      })}
    </AccessStatus>
  );
}
