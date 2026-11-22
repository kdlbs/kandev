"use client";
import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHEnrollment, SSHIdentity } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { AccessHeading, AccessStatus } from "./access-primitives";
import { EnrollmentMethods } from "./enrollment-methods";
import { EnrollmentReview } from "./enrollment-review";
import { EnrollmentStatus } from "./enrollment-status";
import { useEnrollmentFlow } from "./use-enrollment-flow";
import { OperationFeedback } from "./operation-feedback";
export { EnrollmentStatus } from "./enrollment-status";
type EnrollmentProps = {
  route: SSHRoute;
  manager: SSHManager;
  onCompleted?: (result: SSHEnrollment) => void;
};
export function EnrollmentPanel(props: EnrollmentProps) {
  const identity = props.manager.identities.find(
    (item) => item.id === props.route.destination.identity_id,
  );
  if (!identity?.public_key) return null;
  return (
    <EnrollmentForm
      key={`${props.route.stamp}:${identity.fingerprint}`}
      {...props}
      identity={identity}
    />
  );
}
function EnrollmentForm({
  route,
  manager,
  identity,
  onCompleted,
}: EnrollmentProps & { identity: SSHIdentity }) {
  const { t } = useTranslation();
  const flow = useEnrollmentFlow(route, identity, manager, onCompleted);
  const content = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    content.current?.querySelector<HTMLElement>("[data-ssh-step-heading]")?.focus();
  }, [flow.step]);
  if (flow.boundary.changed) return null;
  return (
    <section
      ref={content}
      className="space-y-6"
      aria-label={t("settings:sshManager.enrollment.title")}
      onKeyDown={(event) => {
        if (event.key === "Escape" && flow.step === "review") {
          event.preventDefault();
          flow.back();
        }
      }}
    >
      {flow.step === "choose" && (
        <EnrollmentChoice flow={flow} route={route} manager={manager} identity={identity} />
      )}
      {flow.step === "review" && (
        <>
          <EnrollmentReview
            route={route}
            identity={identity}
            method={flow.method}
            bootstrapLabel={manager.identities.find((item) => item.id === flow.bootstrap)?.label}
          />
          <div className="flex flex-col-reverse gap-2 border-t pt-4 md:flex-row md:justify-end">
            <Button variant="ghost" className={settingsActionClassName()} onClick={flow.back}>
              {t("common:back")}
            </Button>
            <Button
              className={settingsActionClassName()}
              disabled={manager.busy}
              onClick={() => void flow.install()}
            >
              {t("settings:sshManager.enrollment.install")}
            </Button>
          </div>
        </>
      )}
      {flow.step === "pending" && (
        <AccessStatus pending title={t("settings:sshManager.experience.pending.enroll")}>
          {t("settings:sshManager.experience.installPendingHelp")}
        </AccessStatus>
      )}
      {flow.step === "result" && (
        <>
          <AccessHeading title={t("settings:sshManager.experience.installResult")} />
          <EnrollmentStatus latest={flow.result} />
          {flow.failed && (
            <AccessStatus
              tone="warning"
              title={t("settings:sshManager.enrollment.states.outcome_unknown")}
            />
          )}
          <OperationFeedback manager={manager} route={route} kind="test" />
          <div className="flex flex-col gap-2 md:flex-row">
            <Button
              className={settingsActionClassName()}
              disabled={manager.busy}
              onClick={() => void manager.test(route).catch(() => undefined)}
            >
              {t("settings:sshManager.test")}
            </Button>
            {flow.result?.state !== "started" && (
              <Button variant="outline" className={settingsActionClassName()} onClick={flow.back}>
                {t("settings:sshManager.experience.chooseMethod")}
              </Button>
            )}
          </div>
        </>
      )}
    </section>
  );
}

function EnrollmentChoice({
  flow,
  route,
  manager,
  identity,
}: {
  flow: ReturnType<typeof useEnrollmentFlow>;
  route: SSHRoute;
  manager: SSHManager;
  identity: SSHIdentity;
}) {
  const { t } = useTranslation();
  return (
    <form
      className="space-y-6"
      onSubmit={(event) => {
        event.preventDefault();
        flow.review();
      }}
    >
      <AccessHeading
        title={t("settings:sshManager.ui.installKey")}
        description={t("settings:sshManager.experience.chooseMethod")}
      />
      {flow.started ? (
        <EnrollmentStatus latest={route.latest_enrollment} />
      ) : (
        <fieldset disabled={manager.busy} className="space-y-6">
          <EnrollmentMethods
            method={flow.method}
            onMethod={flow.chooseMethod}
            password={flow.password}
            onPassword={flow.setPassword}
            bootstrap={flow.bootstrap}
            onBootstrap={flow.setBootstrap}
            manager={manager}
            publicKey={identity.public_key}
          />
          {!route.destination.fingerprint && (
            <p className="text-sm text-muted-foreground">
              {t("settings:sshManager.enrollment.trustFirst")}
            </p>
          )}
          {flow.method !== "manual" && (
            <div className="flex justify-end border-t pt-4">
              <Button
                type="submit"
                disabled={!flow.valid}
                className={settingsActionClassName("max-md:w-full")}
              >
                {t("settings:sshManager.experience.reviewInstallation")}
              </Button>
            </div>
          )}
        </fieldset>
      )}
    </form>
  );
}
