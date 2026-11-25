"use client";
import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { useConfirmationBoundary } from "@/components/confirmation/mobile-action-confirmation";
import { settingsActionClassName } from "../settings-control";
import { AccessHeading, AccessStatus, EvidenceRow } from "./access-primitives";
import { OperationFeedback, currentOperation, isTrustInUse } from "./operation-feedback";

export function HostTrustReview({
  route,
  manager,
  observation,
  onClose,
  onInspect,
  onNewConnection,
}: {
  route: SSHRoute;
  manager: SSHManager;
  observation: SSHObservation;
  onClose: () => void;
  onInspect: () => void;
  onNewConnection?: () => void;
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
  const expired =
    operation?.kind === "trust" &&
    operation.status === "failed" &&
    operation.error === "ssh_trust_required";
  const inUse = isTrustInUse(operation);
  if (boundary.changed || observation.stamp !== route.stamp) return null;
  const changed = Boolean(
    route.destination.fingerprint && route.destination.fingerprint !== observation.fingerprint,
  );
  return (
    <div
      ref={content}
      className="space-y-4 py-3"
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
      {changed && !inUse && (
        <AccessStatus
          tone="warning"
          title={t("settings:sshManager.experience.changedServer")}
          staticMessage
        >
          {t("settings:sshManager.errors.ssh_host_key_changed")}
        </AccessStatus>
      )}
      <TrustEvidence route={route} observation={observation} changed={changed} />
      <OperationFeedback manager={manager} route={route} kind="trust" changedHost={changed} />
      <div className="flex flex-col-reverse gap-2 border-t pt-4 md:flex-row md:justify-end">
        <Button variant="ghost" className={settingsActionClassName()} onClick={onClose}>
          {t("common:back")}
        </Button>
        <TrustReviewAction
          blocked={inUse && changed}
          expired={expired}
          busy={manager.busy}
          onNewConnection={onNewConnection}
          onReinspect={() => {
            onClose();
            onInspect();
          }}
          onConfirm={() => {
            if (submitted.current) return;
            submitted.current = true;
            void manager
              .trust(route, observation)
              .then(onClose)
              .catch(() => {
                submitted.current = false;
              });
          }}
        />
      </div>
    </div>
  );
}

function TrustEvidence({
  route,
  observation,
  changed,
}: {
  route: SSHRoute;
  observation: SSHObservation;
  changed: boolean;
}) {
  const { t } = useTranslation();
  return (
    <dl className="divide-y divide-border/60">
      <EvidenceRow
        label={t("settings:sshManager.experience.account")}
        value={`${route.destination.user}@${route.destination.host}:${route.destination.port}`}
      />
      <EvidenceRow
        label={t("settings:sshManager.target")}
        value={t("settings:sshManager.destination")}
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
  );
}

function TrustReviewAction({
  blocked,
  expired,
  busy,
  onNewConnection,
  onReinspect,
  onConfirm,
}: {
  blocked: boolean;
  expired: boolean;
  busy: boolean;
  onNewConnection?: () => void;
  onReinspect: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  const recovery = blocked && onNewConnection;
  const disabled = busy || (blocked && !recovery);
  const action = recovery || (expired ? onReinspect : onConfirm);
  let label = t("settings:sshManager.trust");
  if (recovery) label = t("settings:sshManager.newConnection");
  else if (expired) label = t("settings:sshManager.observe");
  return (
    <Button
      className={settingsActionClassName("max-md:w-full aria-disabled:opacity-50")}
      aria-disabled={disabled}
      onClick={() => {
        if (!disabled) action();
      }}
    >
      {label}
    </Button>
  );
}
