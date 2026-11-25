"use client";
import { useLayoutEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { SSHRoute, SSHIdentity } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { AccessHeading, AccessStatus } from "./access-primitives";
import { EnrollmentMethods } from "./enrollment-methods";
import { EnrollmentReview } from "./enrollment-review";
import { EnrollmentStatus } from "./enrollment-status";
import { useEnrollmentFlow } from "./use-enrollment-flow";
export { EnrollmentStatus } from "./enrollment-status";
type EnrollmentProps = {
  route: SSHRoute;
  manager: SSHManager;
  onTestAccess?: () => void;
  onClose?: () => void;
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
  onTestAccess,
  onClose,
}: EnrollmentProps & { identity: SSHIdentity }) {
  const { t } = useTranslation();
  const flow = useEnrollmentFlow(route, identity, manager);
  const content = useRef<HTMLElement>(null);
  const previousStep = useRef(flow.step);
  useLayoutEffect(() => {
    const returning = previousStep.current !== "choose" && flow.step === "choose";
    previousStep.current = flow.step;
    if (flow.step === "pending" || flow.step === "result") return;
    const section = content.current;
    const heading = section?.querySelector<HTMLElement>("[data-ssh-step-heading]");
    const initiator = returning
      ? (section?.querySelector<HTMLElement>('button[type="submit"]:enabled') ??
        section?.querySelector<HTMLElement>("#ssh-enrollment-password"))
      : undefined;
    (initiator ?? heading)?.focus();
  }, [flow.step]);
  if (flow.boundary.changed) return null;
  const close = onClose && (
    <Button
      type="button"
      variant="ghost"
      className={settingsActionClassName("md:order-first")}
      onClick={onClose}
    >
      {t("settings:sshManager.ui.cancelInstall")}
    </Button>
  );
  return (
    <section
      ref={content}
      className="space-y-6"
      onKeyDown={(event) => {
        if (event.key === "Escape" && flow.step === "review") {
          event.preventDefault();
          flow.back();
        }
      }}
    >
      {flow.step === "choose" && (
        <EnrollmentChoice flow={flow} route={route} manager={manager} identity={identity}>
          {close}
        </EnrollmentChoice>
      )}
      {flow.step !== "choose" && (
        <EnrollmentReview
          route={route}
          identity={identity}
          method={flow.method}
          bootstrapLabel={manager.identities.find((item) => item.id === flow.bootstrap)?.label}
          onBack={flow.back}
          onInstall={() => void flow.install()}
          busy={manager.busy || flow.step !== "review"}
          step={flow.step}
          actions={flow.step !== "review" && close}
        >
          {flow.step === "pending" && (
            <AccessStatus pending title={t("settings:sshManager.experience.pending.enroll")}>
              {t("settings:sshManager.experience.installPendingHelp")}
            </AccessStatus>
          )}
          {flow.step === "result" && <EnrollmentResult flow={flow} onTestAccess={onTestAccess} />}
        </EnrollmentReview>
      )}
    </section>
  );
}

function EnrollmentResult({
  flow,
  onTestAccess,
}: {
  flow: ReturnType<typeof useEnrollmentFlow>;
  onTestAccess?: () => void;
}) {
  const { t } = useTranslation();
  const canCheck =
    flow.failed ||
    flow.result?.state === "installed_unverified" ||
    flow.result?.state === "outcome_unknown";
  return (
    <>
      <EnrollmentStatus latest={flow.result} />
      {flow.failed && (
        <AccessStatus
          tone="warning"
          title={t("settings:sshManager.enrollment.states.outcome_unknown")}
        />
      )}
      {canCheck && onTestAccess && (
        <Button className={settingsActionClassName("max-md:w-full")} onClick={onTestAccess}>
          {t("settings:sshManager.test")}
        </Button>
      )}
      {flow.result?.state === "failed" && (
        <Button
          variant="outline"
          className={settingsActionClassName("max-md:w-full")}
          onClick={flow.back}
        >
          {t("settings:sshManager.experience.chooseMethod")}
        </Button>
      )}
    </>
  );
}

function EnrollmentChoice({
  flow,
  route,
  manager,
  identity,
  children,
}: {
  flow: ReturnType<typeof useEnrollmentFlow>;
  route: SSHRoute;
  manager: SSHManager;
  identity: SSHIdentity;
  children?: ReactNode;
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
        <fieldset disabled={manager.busy} className="min-w-0 space-y-6">
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
        </fieldset>
      )}
      <EnrollmentFooter>
        {!flow.started && flow.method !== "manual" && (
          <Button
            type="submit"
            disabled={!flow.valid || manager.busy}
            className={settingsActionClassName()}
          >
            {t("settings:sshManager.experience.reviewInstallation")}
          </Button>
        )}
        {children}
      </EnrollmentFooter>
    </form>
  );
}

function EnrollmentFooter({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 border-t pt-4 md:flex-row md:flex-wrap md:justify-end">
      {children}
    </div>
  );
}
