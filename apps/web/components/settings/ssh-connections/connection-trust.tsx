"use client";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconArrowRight, IconShieldCheck } from "@tabler/icons-react";
import type { SSHRoute, SSHObservation } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { connectionState } from "./connection-presentation";
import { AccessHeading, AccessStatus, EvidenceRow } from "./access-primitives";
import { HostTrustReview } from "./host-trust-review";
import { OperationFeedback, currentOperation } from "./operation-feedback";

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
  const active = useRef(true);
  const currentStamp = useRef(route.stamp);
  currentStamp.current = route.stamp;
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const ready =
    connectionState(route, manager.identities, manager.failedTests?.[route.destination.id]) ===
    "ready";
  const trusted = Boolean(route.destination.fingerprint);
  const operation = currentOperation(manager, route);
  const observeId = `ssh-observe-${route.destination.id}`;
  const close = () => {
    if (!active.current) return;
    setObservation(null);
    requestAnimationFrame(() => document.getElementById(observeId)?.focus());
  };
  if (observation)
    return (
      <HostTrustReview route={route} manager={manager} observation={observation} onClose={close} />
    );
  const inspect = () => {
    void manager
      .observe(route)
      .then((value) => {
        if (active.current && currentStamp.current === value.stamp) setObservation(value);
      })
      .catch(() => undefined);
  };
  return (
    <section className="min-w-0 space-y-6">
      {ready && !operation ? (
        <AccessStatus tone="success" title={t("settings:sshManager.ready")}>
          {t("settings:sshManager.experience.verifiedHelp")}
        </AccessStatus>
      ) : (
        <AccessHeading
          title={t(
            trusted
              ? "settings:sshManager.experience.checkAccess"
              : "settings:sshManager.experience.reviewServer",
          )}
          description={t(
            trusted ? "settings:sshManager.ui.setupTest" : "settings:sshManager.ui.setupTrust",
          )}
        />
      )}
      <OperationFeedback manager={manager} route={route} />
      <div className="flex flex-col gap-2 md:flex-row">
        {trusted && (
          <Button
            id={`ssh-test-${route.destination.id}`}
            className={settingsActionClassName("max-md:w-full")}
            disabled={manager.busy}
            onClick={() => {
              void manager.test(route).catch(() => undefined);
            }}
          >
            <IconShieldCheck aria-hidden className="mr-1.5 size-3.5" />
            {t("settings:sshManager.test")}
          </Button>
        )}
        <Button
          id={observeId}
          variant={trusted ? "outline" : "default"}
          className={settingsActionClassName("max-md:w-full")}
          disabled={manager.busy}
          onClick={inspect}
        >
          {t("settings:sshManager.observe")}
          {!trusted && <IconArrowRight aria-hidden className="ml-1.5 size-3.5" />}
        </Button>
      </div>
      {trusted && <HostDetails route={route} />}
      <p className="text-xs leading-relaxed text-muted-foreground">
        {t("settings:sshManager.executorReadiness")}
      </p>
    </section>
  );
}

function HostDetails({ route }: { route: SSHRoute }) {
  const { t } = useTranslation();
  return (
    <details className="border-t pt-3">
      <summary className="inline-flex min-h-7 cursor-pointer items-center text-xs font-medium text-muted-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        {t("settings:sshManager.experience.hostDetails")}
      </summary>
      <dl>
        <EvidenceRow
          label={t("settings:sshManager.experience.serverFingerprint")}
          value={route.destination.fingerprint}
          copy
        />
      </dl>
    </details>
  );
}
