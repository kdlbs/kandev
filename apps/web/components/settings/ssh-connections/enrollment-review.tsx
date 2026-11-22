"use client";
import { useTranslation } from "react-i18next";
import type { SSHRoute, SSHIdentity } from "@/lib/types/http-ssh-connections";
import { AccessHeading, EvidenceRow } from "./access-primitives";
import type { EnrollmentMethod } from "./enrollment-methods";
export function EnrollmentReview({
  route,
  identity,
  method,
  bootstrapLabel,
}: {
  route: SSHRoute;
  identity: SSHIdentity;
  method: EnrollmentMethod;
  bootstrapLabel?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <AccessHeading
        title={t("settings:sshManager.experience.reviewInstallation")}
        description={t("settings:sshManager.enrollment.confirmHelp")}
      />
      <dl className="divide-y divide-border/60">
        <EvidenceRow
          label={t("settings:sshManager.experience.account")}
          value={`${route.destination.user}@${route.destination.host}:${route.destination.port}`}
        />
        {route.bastion && (
          <EvidenceRow
            label={t("settings:sshManager.bastion")}
            value={`${route.bastion.user}@${route.bastion.host}:${route.bastion.port}`}
          />
        )}
        <EvidenceRow
          label={t("settings:sshManager.experience.serverFingerprint")}
          value={route.destination.fingerprint}
          copy
        />
        <EvidenceRow label={t("settings:sshManager.identity")} value={identity.label} />
        <EvidenceRow
          label={t("settings:sshManager.experience.keyFingerprint")}
          value={identity.fingerprint}
          copy
        />
        <EvidenceRow
          label={t("settings:sshManager.method")}
          value={
            method === "password"
              ? t("settings:sshManager.experience.methodPassword")
              : (bootstrapLabel ?? t("settings:sshManager.experience.methodIdentity"))
          }
        />
      </dl>
    </div>
  );
}
