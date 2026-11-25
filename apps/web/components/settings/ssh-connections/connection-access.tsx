"use client";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { SettingsGroup, SettingsRow } from "../settings-group";
import { settingsActionClassName } from "../settings-control";
import {
  canVerifyOnFirstUse,
  connectionEndpoint,
  connectionState,
} from "./connection-presentation";
import { ConnectionTrust } from "./connection-trust";
import { EnrollmentPanel } from "./enrollment-panel";

type AccessProps = {
  route: SSHRoute;
  manager: SSHManager;
  onEdit: () => void;
  onBastion?: () => void;
  onNewConnection?: () => void;
  onSetupChecks?: () => void;
  setupChecks?: ReactNode;
};

export function ConnectionAccess({
  route,
  manager,
  onEdit,
  onBastion,
  onNewConnection,
  onSetupChecks,
  setupChecks,
}: AccessProps) {
  const [observation, setObservation] = useState<SSHObservation | null>(null);
  const [enrolling, setEnrolling] = useState(false);
  const identity = manager.identities.find((item) => item.id === route.destination.identity_id);
  const latest =
    route.latest_enrollment?.stamp === route.stamp ? route.latest_enrollment : undefined;
  const canInstall = Boolean(identity?.public_key && route.destination.fingerprint);
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
  const id = route.destination.id;
  const repairReason = route.destination.repair_reason;
  const missingDetails = state === "incomplete" || state === "unavailable" || Boolean(repairReason);
  const missingBastion = needsBastionSetup(route, manager);
  const enrollment = enrolling && canInstall && (
    <EnrollmentPanel
      route={route}
      manager={manager}
      onTestAccess={() => {
        if (manager.busy) return;
        close(`ssh-test-${id}`);
        void manager.test(route).catch(() => undefined);
      }}
      onClose={() => close(`ssh-install-${id}`)}
    />
  );
  return (
    <div className="space-y-6">
      {missingDetails || missingBastion ? (
        <>
          <AccessPrerequisite
            missingDetails={missingDetails}
            onEdit={onEdit}
            onBastion={onBastion}
            bastion={route.bastion}
            repairReason={repairReason}
            imported={canVerifyOnFirstUse(route)}
          />
          {setupChecks}
        </>
      ) : (
        <ConnectionTrust
          route={route}
          manager={manager}
          observation={observation}
          setObservation={setObservation}
          latest={identity?.public_key ? latest : undefined}
          enrollment={enrollment}
          setupChecks={setupChecks}
          onInstall={canInstall ? () => setEnrolling(true) : undefined}
          onNewConnection={onNewConnection}
          onSetupChecks={onSetupChecks}
        />
      )}
    </div>
  );
}

function AccessPrerequisite({
  missingDetails,
  onEdit,
  onBastion,
  bastion,
  repairReason,
  imported,
}: Pick<AccessProps, "onEdit" | "onBastion"> & {
  missingDetails: boolean;
  bastion?: SSHRoute["bastion"];
  repairReason?: string;
  imported: boolean;
}) {
  const { t } = useTranslation();
  let description = t("settings:sshManager.errors.ssh_bastion_not_ready");
  if (imported) description = t("settings:sshManager.binding.imported");
  if (missingDetails) description = t("settings:sshManager.ui.setupDetails");
  return (
    <SettingsGroup title={t("settings:sshManager.ui.setupAccess")}>
      <SettingsRow
        label={t(
          missingDetails ? "settings:sshManager.experience.details" : "settings:sshManager.bastion",
        )}
        description={
          <span className="space-y-1">
            {!missingDetails && bastion && (
              <span className="block break-all">{connectionEndpoint(bastion)}</span>
            )}
            <span className="block">
              {repairReason
                ? t(`settings:sshManager.errors.${repairReason}`, { defaultValue: description })
                : description}
            </span>
          </span>
        }
        control={
          <Button
            variant="outline"
            className={settingsActionClassName("max-md:w-full")}
            onClick={missingDetails ? onEdit : onBastion}
          >
            {t(missingDetails ? "common:edit" : "settings:sshManager.binding.reviewBastion")}
          </Button>
        }
      />
    </SettingsGroup>
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
