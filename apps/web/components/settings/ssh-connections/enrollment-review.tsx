"use client";
import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { settingsActionClassName } from "../settings-control";
import { useTranslation } from "react-i18next";
import type { SSHRoute, SSHIdentity } from "@/lib/types/http-ssh-connections";
import { AccessHeading, EvidenceRow } from "./access-primitives";
import type { EnrollmentMethod } from "./enrollment-methods";
export function EnrollmentReview({
  route,
  identity,
  method,
  bootstrapLabel,
  onBack,
  onInstall,
  busy,
  step,
  actions,
  children,
}: {
  route: SSHRoute;
  identity: SSHIdentity;
  method: EnrollmentMethod;
  bootstrapLabel?: string;
  onBack: () => void;
  onInstall: () => void;
  busy: boolean;
  step: "review" | "pending" | "result";
  actions?: ReactNode;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <section className="space-y-4" aria-label={t("settings:sshManager.enrollment.title")}>
      <AccessHeading
        title={t("settings:sshManager.experience.reviewInstallation")}
        description={
          step === "review" ? t("settings:sshManager.enrollment.confirmHelp") : undefined
        }
      />
      <dl className="divide-y divide-border/60" hidden={step !== "review"}>
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
      <div className="flex flex-col-reverse gap-2 border-t pt-4 md:flex-row md:flex-wrap md:justify-end">
        <Button
          variant="ghost"
          className={settingsActionClassName(step === "review" ? undefined : "hidden")}
          onClick={onBack}
          hidden={step !== "review"}
        >
          {t("common:back")}
        </Button>
        <Button
          className={settingsActionClassName("aria-disabled:opacity-50")}
          aria-disabled={busy}
          onClick={onInstall}
        >
          {t("settings:sshManager.enrollment.install")}
        </Button>
        {actions}
      </div>
      {children}
    </section>
  );
}
