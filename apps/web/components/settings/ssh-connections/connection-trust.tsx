"use client";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHEnrollment, SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { SettingsGroup, SettingsRow } from "../settings-group";
import { connectionState } from "./connection-presentation";
import { AccessStatus, EvidenceRow } from "./access-primitives";
import { HostTrustReview } from "./host-trust-review";
import { OperationFeedback, currentOperation } from "./operation-feedback";
import { EnrollmentStatus } from "./enrollment-status";

type TrustProps = {
  route: SSHRoute;
  manager: SSHManager;
  observation: SSHObservation | null;
  setObservation: (value: SSHObservation | null) => void;
  latest?: SSHEnrollment;
  enrollment?: ReactNode;
  setupChecks?: ReactNode;
  onInstall?: () => void;
  onNewConnection?: () => void;
  onSetupChecks?: () => void;
};

export function ConnectionTrust({
  route,
  manager,
  observation,
  setObservation,
  latest,
  enrollment,
  setupChecks,
  onInstall,
  onNewConnection,
  onSetupChecks,
}: TrustProps) {
  const { t } = useTranslation();
  const active = useRef(true);
  const currentStamp = useRef(route.stamp);
  currentStamp.current = route.stamp;
  const [details, setDetails] = useState(false);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const observeId = `ssh-observe-${route.destination.id}`;
  const close = () => {
    if (!active.current) return;
    setObservation(null);
    requestAnimationFrame(() => document.getElementById(observeId)?.focus());
  };
  const inspect = () => {
    if (manager.busy) return;
    void manager
      .observe(route)
      .then((value) => {
        if (active.current && currentStamp.current === value.stamp) {
          setDetails(true);
          setObservation(value);
        }
      })
      .catch(() => undefined);
  };
  const trusted = Boolean(route.destination.fingerprint);
  const operation = currentOperation(manager, route);
  const hostChanged = operation?.kind === "test" && operation.error === "ssh_host_key_changed";
  const inspecting = operation?.kind === "observe";
  const trust = (
    <SettingsGroup title={t("settings:sshManager.experience.serverTrust")} frame="none">
      {observation ? (
        <HostTrustReview
          route={route}
          manager={manager}
          observation={observation}
          onClose={close}
          onInspect={inspect}
          onNewConnection={onNewConnection}
        />
      ) : (
        <TrustRow
          route={route}
          manager={manager}
          details={details}
          activeInspection={!trusted || hostChanged || inspecting}
          onToggleDetails={() => setDetails(!details)}
          onInspect={inspect}
          observeId={observeId}
        />
      )}
    </SettingsGroup>
  );
  if (observation) return trust;
  if (enrollment) {
    return (
      <SettingsGroup title={t("settings:sshManager.experience.sshAccess")} frame="none">
        {enrollment}
      </SettingsGroup>
    );
  }
  return (
    <div className="flex flex-col gap-6">
      <div
        className={trusted && !hostChanged && !inspecting ? "order-last md:order-none" : undefined}
      >
        {trust}
      </div>
      <div className="grid items-start gap-6 @min-[54rem]/ssh-detail:grid-cols-2">
        {trusted && (
          <LoginRow
            route={route}
            manager={manager}
            latest={latest}
            onInstall={onInstall}
            onSetupChecks={onSetupChecks}
          />
        )}
        {setupChecks}
      </div>
    </div>
  );
}

function TrustRow({
  route,
  manager,
  details,
  activeInspection,
  onToggleDetails,
  onInspect,
  observeId,
}: Pick<TrustProps, "route" | "manager"> & {
  details: boolean;
  activeInspection: boolean;
  onToggleDetails: () => void;
  onInspect: () => void;
  observeId: string;
}) {
  const { t } = useTranslation();
  const trusted = Boolean(route.destination.fingerprint);
  return (
    <>
      <SettingsRow
        label={t(
          trusted
            ? "settings:sshManager.experience.trustedServer"
            : "settings:sshManager.ui.states.untrusted",
        )}
        control={
          <div className="flex flex-wrap gap-2 max-md:w-full">
            {trusted && (
              <Button
                type="button"
                variant="outline"
                className={settingsActionClassName("max-md:w-full")}
                aria-expanded={details}
                onClick={onToggleDetails}
              >
                {t("settings:sshManager.experience.hostDetails")}
              </Button>
            )}
            {(activeInspection || details) && (
              <ObserveButton
                id={observeId}
                busy={manager.busy}
                onClick={onInspect}
                secondary={trusted}
              />
            )}
          </div>
        }
      />
      {trusted && details && (
        <div className="space-y-3 border-t border-border/70 py-3">
          <dl>
            <EvidenceRow
              label={t("settings:sshManager.experience.serverFingerprint")}
              value={route.destination.fingerprint}
              copy
            />
          </dl>
        </div>
      )}
      <OperationFeedback manager={manager} route={route} kind="observe" />
    </>
  );
}

function ObserveButton({
  id,
  busy,
  secondary,
  onClick,
}: {
  id: string;
  busy: boolean;
  secondary?: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      id={id}
      variant={secondary ? "outline" : "default"}
      className={settingsActionClassName("max-md:w-full aria-disabled:opacity-50")}
      aria-disabled={busy}
      onClick={onClick}
    >
      {t("settings:sshManager.observe")}
    </Button>
  );
}

function LoginRow({
  route,
  manager,
  latest,
  onInstall,
  onSetupChecks,
}: Pick<TrustProps, "route" | "manager" | "latest" | "onInstall" | "onSetupChecks">) {
  const { t } = useTranslation();
  const state = connectionState(
    route,
    manager.identities,
    manager.failedTests?.[route.destination.id],
  );
  const operation = currentOperation(manager, route);
  const test = operation?.kind === "test" ? operation : null;
  const rejected = test?.status === "failed" && test.error === "ssh_authentication_rejected";
  const ready = state === "ready" && !test;
  return (
    <SettingsGroup
      title={t("settings:sshManager.experience.sshAccess")}
      description={
        test || ready
          ? t("settings:sshManager.experience.liveTestHelp")
          : t(`settings:sshManager.ui.states.${state}`)
      }
      frame="none"
      contentClassName="space-y-3"
      action={
        <Button
          id={`ssh-test-${route.destination.id}`}
          className={settingsActionClassName("max-md:w-full aria-disabled:opacity-50")}
          aria-disabled={manager.busy}
          onClick={() => {
            if (!manager.busy) void manager.test(route).catch(() => undefined);
          }}
        >
          {t("settings:sshManager.test")}
        </Button>
      }
    >
      <OperationFeedback manager={manager} route={route} kind="test" />
      <SavedAccessStatus latest={latest} ready={ready} />
      <LoginRemedies
        id={route.destination.id}
        installDisabled={manager.busy || latest?.state === "started"}
        onInstall={onInstall}
        onSetupChecks={rejected ? onSetupChecks : undefined}
      />
    </SettingsGroup>
  );
}

function SavedAccessStatus({ latest, ready }: { latest?: SSHEnrollment; ready: boolean }) {
  const { t } = useTranslation();
  if (latest?.state === "verified" && !ready) return null;
  if (ready && latest?.state !== "verified") {
    return (
      <>
        <AccessStatus tone="success" title={t("settings:sshManager.ready")} staticMessage>
          {t("settings:sshManager.experience.verifiedHelp")}
        </AccessStatus>
        <EnrollmentStatus latest={latest} staticMessage />
      </>
    );
  }
  return <EnrollmentStatus latest={latest} staticMessage />;
}

function LoginRemedies({
  id,
  installDisabled,
  onInstall,
  onSetupChecks,
}: Pick<TrustProps, "onInstall" | "onSetupChecks"> & { id: string; installDisabled: boolean }) {
  const { t } = useTranslation();
  if (!onInstall && !onSetupChecks) return null;
  return (
    <div className="flex flex-col gap-2 py-2 md:flex-row md:flex-wrap">
      {onInstall && (
        <Button
          id={`ssh-install-${id}`}
          variant="outline"
          className={settingsActionClassName()}
          disabled={installDisabled}
          onClick={onInstall}
        >
          {t("settings:sshManager.ui.installKey")}
        </Button>
      )}
      {onSetupChecks && (
        <Button variant="outline" className={settingsActionClassName()} onClick={onSetupChecks}>
          {t("settings:sshManager.experience.setup")}
        </Button>
      )}
    </div>
  );
}
