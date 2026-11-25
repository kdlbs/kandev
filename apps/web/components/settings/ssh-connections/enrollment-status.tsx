"use client";
import { useTranslation } from "react-i18next";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import { AccessStatus } from "./access-primitives";
export function EnrollmentStatus({
  latest,
  staticMessage,
}: {
  latest: SSHRoute["latest_enrollment"];
  staticMessage?: boolean;
}) {
  const { t } = useTranslation();
  if (!latest) return null;
  let tone: "success" | "error" | "warning" = "warning";
  if (latest.state === "verified") tone = "success";
  if (latest.state === "failed") tone = "error";
  return (
    <AccessStatus
      tone={tone}
      staticMessage={staticMessage}
      pending={latest.state === "started"}
      title={t(`settings:sshManager.enrollment.states.${latest.state}`)}
    >
      {latest.error_code && (
        <p>
          {latest.hop && <>{t(`settings:sshManager.${latest.hop}`)}: </>}
          {t(`settings:sshManager.errors.${latest.error_code}`, {
            defaultValue: t("settings:sshManager.errors.ssh_operation_failed"),
          })}
        </p>
      )}
    </AccessStatus>
  );
}
