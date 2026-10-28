"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { SettingsGroup } from "../settings-group";
import { settingsActionClassName } from "../settings-control";
import { connectionState } from "./connection-presentation";
import { ConnectionTrust } from "./connection-trust";
import { EnrollmentPanel, EnrollmentStatus } from "./enrollment-panel";
import { PublicKeyDetails } from "./public-key-details";

export function ConnectionAccess({
  route,
  manager,
  onEdit,
}: {
  route: SSHRoute;
  manager: SSHManager;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const [observation, setObservation] = useState<SSHObservation | null>(null);

  const state = connectionState(
    route,
    manager.identities,
    manager.failedTests?.[route.destination.id],
  );
  if (state === "incomplete" || state === "unavailable")
    return (
      <SettingsGroup title={t("settings:sshManager.ui.access")}>
        <div className="py-4 space-y-4">
          <p className="text-sm text-muted-foreground">
            {t("settings:sshManager.ui.setupDetails")}
          </p>
          <Button variant="outline" className={settingsActionClassName()} onClick={onEdit}>
            {t("common:edit")}
          </Button>
        </div>
      </SettingsGroup>
    );
  return (
    <div className="space-y-8">
      <SettingsGroup
        title={t("settings:sshManager.ui.access")}
        description={t(
          state === "untrusted"
            ? "settings:sshManager.ui.setupTrust"
            : "settings:sshManager.ui.setupTest",
        )}
      >
        <div className="py-4">
          <ConnectionTrust
            route={route}
            manager={manager}
            observation={observation}
            setObservation={setObservation}
          />
        </div>
      </SettingsGroup>
      <ConnectionAuthorization route={route} manager={manager} />
    </div>
  );
}

function ConnectionAuthorization({ route, manager }: { route: SSHRoute; manager: SSHManager }) {
  const { t } = useTranslation();
  const [enrolling, setEnrolling] = useState(false);
  const identity = manager.identities.find((item) => item.id === route.destination.identity_id);
  const latest =
    route.latest_enrollment?.stamp === route.stamp ? route.latest_enrollment : undefined;
  const close = (focusId: string) => {
    setEnrolling(false);
    requestAnimationFrame(() => document.getElementById(focusId)?.focus());
  };
  if (!identity?.public_key) return null;
  return (
    <SettingsGroup title={t("settings:sshManager.enrollment.title")}>
      <div className="py-4 space-y-5">
        {!enrolling && (
          <>
            <EnrollmentStatus latest={latest} />
            <PublicKeyDetails publicKey={identity.public_key} />
            {route.destination.fingerprint && (
              <Button
                id={`ssh-install-${route.destination.id}`}
                variant="outline"
                className={settingsActionClassName()}
                disabled={manager.busy || latest?.state === "started"}
                onClick={() => setEnrolling(true)}
              >
                {t("settings:sshManager.ui.installKey")}
              </Button>
            )}
          </>
        )}
        {enrolling && (
          <>
            <EnrollmentPanel
              route={route}
              manager={manager}
              onCompleted={() => close(`ssh-test-${route.destination.id}`)}
            />
            <Button
              variant="ghost"
              className={settingsActionClassName()}
              onClick={() => close(`ssh-install-${route.destination.id}`)}
            >
              {t("settings:sshManager.ui.cancelInstall")}
            </Button>
          </>
        )}
      </div>
    </SettingsGroup>
  );
}
