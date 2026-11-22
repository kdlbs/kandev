"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHPreflight } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { AccessHeading, AccessStatus, AccessSurface } from "./access-primitives";
import { OperationFeedback } from "./operation-feedback";
import { DiagnosticIssues, type RemedyAction } from "./diagnostic-issues";
export function PrerequisitePanel({
  route,
  manager,
  onRemedy,
}: {
  route: SSHRoute;
  manager: SSHManager;
  onRemedy?: RemedyAction;
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
    <div className="max-w-3xl space-y-6">
      <AccessSurface className="space-y-6">
        <AccessHeading
          title={t("settings:sshManager.experience.setup")}
          description={t("settings:sshManager.experience.setupScope")}
        />
        <Button
          className={settingsActionClassName("max-md:w-full")}
          disabled={manager.busy || current?.state === "pending"}
          onClick={() => void check()}
        >
          {t("settings:sshManager.preflight")}
        </Button>
        {current?.state === "pending" && (
          <AccessStatus pending title={t("settings:sshManager.experience.pending.preflight")} />
        )}
        {current?.state === "failed" && (
          <AccessStatus tone="error" title={t("settings:sshManager.experience.failed.preflight")}>
            {t("settings:sshManager.errors.ssh_operation_failed")}
          </AccessStatus>
        )}
        {current?.result && (
          <div className="space-y-5 border-t pt-5">
            <AccessStatus
              tone={current.result.checks.length ? "warning" : "neutral"}
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
      </AccessSurface>
      <section className="space-y-4 px-1">
        <AccessHeading
          title={t("settings:sshManager.experience.liveTest")}
          description={t("settings:sshManager.experience.liveTestHelp")}
        />
        <OperationFeedback manager={manager} route={route} kind="test" />
        <Button
          variant="outline"
          className={settingsActionClassName("max-md:w-full")}
          disabled={manager.busy}
          onClick={() => void manager.test(route).catch(() => undefined)}
        >
          {t("settings:sshManager.test")}
        </Button>
        <p className="text-xs leading-relaxed text-muted-foreground">
          {t("settings:sshManager.executorReadiness")}
        </p>
      </section>
    </div>
  );
}
