"use client";
import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { useConfirmationBoundary } from "@/components/confirmation/mobile-action-confirmation";
import { settingsActionClassName } from "../settings-control";
import { AccessHeading, AccessStatus, EvidenceRow } from "./access-primitives";
import { OperationFeedback, currentOperation } from "./operation-feedback";

export function HostTrustReview({
  route,
  manager,
  observation,
  onClose,
}: {
  route: SSHRoute;
  manager: SSHManager;
  observation: SSHObservation;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const submitted = useRef(false);
  const content = useRef<HTMLDivElement>(null);
  const boundary = useConfirmationBoundary(
    true,
    `${route.destination.id}:${route.stamp}:${observation.receipt}`,
    () => onClose(),
  );
  useLayoutEffect(() => {
    content.current?.querySelector<HTMLElement>("[data-ssh-step-heading]")?.focus();
  }, []);
  useLayoutEffect(() => {
    if (observation.stamp !== route.stamp) onClose();
  }, [observation.stamp, route.stamp, onClose]);
  const operation = currentOperation(manager, route);
  const expired = operation?.kind === "trust" && operation.status === "failed";
  if (boundary.changed || observation.stamp !== route.stamp) return null;
  const changed = Boolean(
    route.destination.fingerprint && route.destination.fingerprint !== observation.fingerprint,
  );
  return (
    <div
      ref={content}
      className="space-y-6"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          onClose();
        }
      }}
    >
      <AccessHeading
        title={t("settings:sshManager.experience.reviewServer")}
        description={t("settings:sshManager.trustDescription")}
      />
      {changed && (
        <AccessStatus tone="warning" title={t("settings:sshManager.experience.changedServer")}>
          {t("settings:sshManager.errors.ssh_host_key_changed")}
        </AccessStatus>
      )}
      <dl className="divide-y divide-border/60">
        <EvidenceRow
          label={t("settings:sshManager.experience.account")}
          value={`${route.destination.user}@${route.destination.host}:${route.destination.port}`}
        />
        {changed && (
          <EvidenceRow
            label={t("settings:sshManager.oldFingerprint")}
            value={route.destination.fingerprint}
            copy
          />
        )}
        <EvidenceRow
          label={t(
            changed
              ? "settings:sshManager.newFingerprint"
              : "settings:sshManager.experience.serverFingerprint",
          )}
          value={observation.fingerprint}
          copy
        />
      </dl>
      <OperationFeedback manager={manager} route={route} kind="trust" />
      <div className="flex flex-col-reverse gap-2 border-t pt-4 md:flex-row md:justify-end">
        <Button variant="ghost" className={settingsActionClassName()} onClick={onClose}>
          {t("common:back")}
        </Button>
        <Button
          className={settingsActionClassName("max-md:w-full")}
          disabled={manager.busy || expired}
          onClick={() => {
            if (submitted.current) return;
            submitted.current = true;
            void manager
              .trust(route, observation)
              .then(onClose)
              .catch(() => {
                submitted.current = false;
              });
          }}
        >
          {t("settings:sshManager.trust")}
        </Button>
      </div>
    </div>
  );
}
