"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { ManagerConfirmation } from "./confirmation";
export function ConnectionTrust({
  route,
  manager,
  observation,
  setObservation,
}: {
  route: SSHRoute;
  manager: SSHManager;
  observation: SSHObservation | null;
  setObservation: (value: SSHObservation | null) => void;
}) {
  const { t } = useTranslation();
  const ready =
    route.destination.verified_route === route.stamp &&
    manager.failedTests?.[route.destination.id] !== route.stamp;
  return (
    <section className="space-y-4 min-w-0">
      <p role="status" className="text-sm font-medium">
        {ready ? t("settings:sshManager.ready") : t("settings:sshManager.draft")}
      </p>
      {route.destination.fingerprint && (
        <p className="break-all font-mono text-xs">{route.destination.fingerprint}</p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant={route.destination.fingerprint ? "outline" : "default"}
          className={settingsActionClassName()}
          disabled={manager.busy}
          onClick={() => {
            void manager
              .observe(route)
              .then(setObservation)
              .catch(() => undefined);
          }}
        >
          {t("settings:sshManager.observe")}
        </Button>
        {route.destination.fingerprint && (
          <Button
            id={`ssh-test-${route.destination.id}`}
            type="button"
            className={settingsActionClassName()}
            disabled={manager.busy || !route.destination.fingerprint}
            onClick={() => {
              void manager.test(route).catch(() => undefined);
            }}
          >
            {t("settings:sshManager.test")}
          </Button>
        )}
      </div>
      {!route.destination.fingerprint && (
        <p className="text-sm text-muted-foreground">{t("settings:sshManager.trustNeeded")}</p>
      )}
      {observation && (
        <div className="space-y-3 rounded-md border p-3">
          <p className="text-sm">{t("settings:sshManager.trustDescription")}</p>
          <p className="break-all font-mono text-xs">{observation.fingerprint}</p>
          <ManagerConfirmation
            label={t("settings:sshManager.trust")}
            title={t("settings:sshManager.trustTitle")}
            description={
              <div className="space-y-3">
                <p>{t("settings:sshManager.trustDescription")}</p>
                {route.destination.fingerprint && (
                  <p className="break-all">
                    {t("settings:sshManager.oldFingerprint")}: {route.destination.fingerprint}
                  </p>
                )}
                <p className="break-all">
                  {t("settings:sshManager.newFingerprint")}: {observation.fingerprint}
                </p>
              </div>
            }
            target={observation.receipt}
            busy={manager.busy}
            onConfirm={async () => {
              await manager.trust(route, observation);
              setObservation(null);
            }}
          />
        </div>
      )}
      <p className="text-sm text-muted-foreground">{t("settings:sshManager.executorReadiness")}</p>
    </section>
  );
}
