"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHPreflight } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { SettingsGroup } from "../settings-group";
import { AccessStatus } from "./access-primitives";
import { DiagnosticIssues, type RemedyAction } from "./diagnostic-issues";
const setupHeadingId = "ssh-setup-checks-heading";

export function focusSetupChecks() {
  document.getElementById(setupHeadingId)?.focus();
}

export function PrerequisitePanel({
  route,
  manager,
  onRemedy,
  focusOnOpen,
}: {
  route: SSHRoute;
  manager: SSHManager;
  onRemedy?: RemedyAction;
  focusOnOpen?: boolean;
}) {
  const { t } = useTranslation();
  const [report, setReport] = useState<{
    stamp: string;
    state: "pending" | "failed" | "done";
    result?: SSHPreflight;
  }>();
  const active = useRef(true);
  const currentStamp = useRef(route.stamp);
  currentStamp.current = route.stamp;
  const pending = useRef(false);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  useEffect(() => {
    if (focusOnOpen) focusSetupChecks();
  }, [focusOnOpen]);
  const check = async () => {
    if (pending.current || manager.busy) return;
    pending.current = true;
    const stamp = route.stamp;
    setReport({ stamp, state: "pending" });
    try {
      const result = await manager.preflight(route);
      if (active.current && currentStamp.current === stamp && result.stamp === stamp)
        setReport({ stamp, state: "done", result });
    } catch {
      if (active.current && currentStamp.current === stamp) setReport({ stamp, state: "failed" });
    } finally {
      pending.current = false;
    }
  };
  const current = report?.stamp === route.stamp ? report : undefined;
  return (
    <SettingsGroup
      title={
        <span id={setupHeadingId} tabIndex={-1} className="outline-none">
          {t("settings:sshManager.experience.setup")}
        </span>
      }
      description={t("settings:sshManager.experience.setupScope")}
      frame="none"
      action={
        <Button
          variant="outline"
          className={settingsActionClassName("max-md:w-full")}
          disabled={manager.busy || current?.state === "pending"}
          onClick={() => void check()}
        >
          {t("settings:sshManager.preflight")}
        </Button>
      }
    >
      {current?.state === "pending" && (
        <AccessStatus pending title={t("settings:sshManager.experience.pending.preflight")} />
      )}
      {current?.state === "failed" && (
        <AccessStatus tone="error" title={t("settings:sshManager.experience.failed.preflight")}>
          {t("settings:sshManager.errors.ssh_operation_failed")}
        </AccessStatus>
      )}
      {current?.result && (
        <div className="space-y-4">
          <AccessStatus
            tone={current.result.checks.length ? "warning" : "success"}
            title={
              current.result.checks.length
                ? t("settings:sshManager.experience.issueCount", {
                    count: current.result.checks.length,
                  })
                : t("settings:sshManager.experience.noSetupIssues")
            }
          />
          <DiagnosticIssues checks={current.result.checks} onRemedy={onRemedy} />
        </div>
      )}
    </SettingsGroup>
  );
}
