"use client";
import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { connectionState } from "./connection-presentation";
import { ConnectionTrust } from "./connection-trust";
import { EnrollmentPanel, EnrollmentStatus } from "./enrollment-panel";
import { AccessProgress } from "./access-progress";
import { AccessHeading, AccessSurface } from "./access-primitives";

export function ConnectionAccess({
  route,
  manager,
  onEdit,
  onBastion,
}: {
  route: SSHRoute;
  manager: SSHManager;
  onEdit: () => void;
  onBastion?: () => void;
}) {
  const { t } = useTranslation();
  const [observation, setObservation] = useState<SSHObservation | null>(null);
  const [enrolling, setEnrolling] = useState(false);
  const identity = manager.identities.find((item) => item.id === route.destination.identity_id);
  const state = connectionState(
    route,
    manager.identities,
    manager.failedTests?.[route.destination.id],
  );
  const focusReturn = useRef<string | null>(null);
  useLayoutEffect(() => {
    if (!enrolling && focusReturn.current) {
      document.getElementById(focusReturn.current)?.focus();
      focusReturn.current = null;
    }
  }, [enrolling]);
  const close = (focusId: string) => {
    focusReturn.current = focusId;
    setEnrolling(false);
  };
  if (state === "incomplete" || state === "unavailable")
    return (
      <div className="max-w-3xl space-y-6">
        <AccessProgress state={state} />
        <AccessSurface className="space-y-5">
          <AccessHeading
            title={t("settings:sshManager.experience.details")}
            description={t("settings:sshManager.ui.setupDetails")}
          />
          <Button variant="outline" className={settingsActionClassName()} onClick={onEdit}>
            {t("common:edit")}
          </Button>
        </AccessSurface>
      </div>
    );
  if (needsBastionSetup(route, manager)) return <BastionSetup onOpen={onBastion} />;
  if (enrolling)
    return (
      <div className="max-w-3xl space-y-4">
        <AccessSurface>
          <EnrollmentPanel
            route={route}
            manager={manager}
            onCompleted={() => close(`ssh-test-${route.destination.id}`)}
          />
        </AccessSurface>
        <Button
          variant="ghost"
          className={settingsActionClassName()}
          onClick={() => close(`ssh-install-${route.destination.id}`)}
        >
          {t("settings:sshManager.ui.cancelInstall")}
        </Button>
      </div>
    );
  return (
    <div className="max-w-3xl space-y-6">
      <AccessProgress state={observation ? "untrusted" : state} />
      <AccessSurface>
        <ConnectionTrust
          route={route}
          manager={manager}
          observation={observation}
          setObservation={setObservation}
        />
      </AccessSurface>
      {identity?.public_key && (
        <InstallationEntry route={route} manager={manager} onInstall={() => setEnrolling(true)} />
      )}
    </div>
  );
}

function needsBastionSetup(route: SSHRoute, manager: SSHManager) {
  if (!route.bastion) return false;
  const bastion = manager.routes?.find((item) => item.destination.id === route.bastion?.id);
  return (
    !bastion ||
    connectionState(bastion, manager.identities, manager.failedTests?.[bastion.destination.id]) !==
      "ready"
  );
}
function BastionSetup({ onOpen }: { onOpen?: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="max-w-3xl space-y-6">
      <AccessProgress state="untrusted" />
      <AccessSurface className="space-y-5">
        <AccessHeading
          title={t("settings:sshManager.bastion")}
          description={t("settings:sshManager.errors.ssh_bastion_not_ready")}
        />
        <Button className={settingsActionClassName()} onClick={onOpen}>
          {t("settings:sshManager.experience.remedyAccess")}
        </Button>
      </AccessSurface>
    </div>
  );
}
function InstallationEntry({
  route,
  manager,
  onInstall,
}: {
  route: SSHRoute;
  manager: SSHManager;
  onInstall: () => void;
}) {
  const { t } = useTranslation();
  const latest =
    route.latest_enrollment?.stamp === route.stamp ? route.latest_enrollment : undefined;
  return (
    <div className="space-y-4">
      <EnrollmentStatus latest={latest} />
      {route.destination.fingerprint && (
        <div className="flex flex-col gap-3 border-t pt-5 md:flex-row md:items-center md:justify-between">
          <p className="text-sm text-muted-foreground">
            {t("settings:sshManager.experience.optionalInstall")}
          </p>
          <Button
            id={`ssh-install-${route.destination.id}`}
            variant="outline"
            className={settingsActionClassName("shrink-0")}
            disabled={manager.busy || latest?.state === "started"}
            onClick={onInstall}
          >
            {t("settings:sshManager.ui.installKey")}
          </Button>
        </div>
      )}
    </div>
  );
}
