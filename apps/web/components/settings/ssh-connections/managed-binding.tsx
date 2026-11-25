"use client";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { useSSHConnections } from "@/hooks/domains/settings/use-ssh-connections";
import { testSSHConnection } from "@/lib/api/domains/ssh-api";
import type { SSHTestResult } from "@/lib/types/http-ssh";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import type { SSHConnectionCardProps } from "../ssh-connection-card";
import { SettingsPageTemplate } from "../settings-page-template";
import { settingsActionClassName, settingsControlClassName } from "../settings-control";
import { AccessStatus } from "./access-primitives";
import { canVerifyOnFirstUse, connectionEndpoint } from "./connection-presentation";
import { ConnectionPicker } from "./picker";
import { flushSync } from "react-dom";
import { useSettingsSaveCoordinator } from "../settings-save-provider";
import { useSSHSetupContinuation, type SSHSetupDraft } from "./setup-continuation";
import { sshConnectionSettingsPath } from "@/lib/settings/ssh-connection-settings-routes";
export type ManagedBindingProps = Pick<
  SSHConnectionCardProps,
  "initial" | "onSave" | "coordinatedSaveId"
> & { ownerUserId?: string };
export function ManagedBinding(props: ManagedBindingProps) {
  const { t } = useTranslation();
  const manager = useSSHConnections();
  const binding = useManagedBinding(props, manager);
  const {
    name,
    setName,
    selected,
    setSelected,
    checked,
    clearCheck,
    busy,
    route,
    canTest,
    canSave,
    dirty,
    suspended,
    conflict,
    manage,
    test,
    save,
    discard,
  } = binding;
  return (
    <SettingsPageTemplate
      title={t("settings:sshManager.binding.title")}
      showPageChrome={false}
      saveId={props.coordinatedSaveId}
      saveRevision={`${name}:${selected}:${route?.stamp ?? ""}:${checked}`}
      isDirty={Boolean(props.coordinatedSaveId) && dirty && !suspended}
      canSave={canSave}
      saveStatus={busy ? "loading" : "idle"}
      onSave={save}
      onDiscard={discard}
    >
      <div className="space-y-4 min-w-0">
        <div className="space-y-1">
          <Label htmlFor="managed-executor-name">{t("settings:sshManager.label")}</Label>
          <Input
            id="managed-executor-name"
            className={settingsControlClassName()}
            value={name}
            disabled={busy}
            onChange={(event) => setName(event.target.value)}
          />
        </div>
        <BindingPicker
          selected={selected}
          manager={manager}
          busy={busy}
          onChange={(value) => {
            setSelected(value);
            clearCheck();
          }}
        />
        <Button
          type="button"
          variant="outline"
          className={settingsActionClassName("max-md:w-full")}
          onClick={() => manage()}
          disabled={busy || manager.busy || conflict}
        >
          {t("settings:sshManager.binding.manage")}
        </Button>
        {conflict && <DraftConflict onDiscard={discard} />}
        <BindingFeedback binding={binding} manager={manager} />
        <BindingActions
          busy={busy}
          canTest={canTest && !conflict}
          canSave={canSave}
          coordinated={Boolean(props.coordinatedSaveId)}
          test={test}
          save={save}
        />
      </div>
    </SettingsPageTemplate>
  );
}

function BindingFeedback({
  binding,
  manager,
}: {
  binding: ReturnType<typeof useManagedBinding>;
  manager: ReturnType<typeof useSSHConnections>;
}) {
  const { t } = useTranslation();
  const { route, ready, canTest, check, busy, conflict, error, manage } = binding;
  const testFailed =
    check?.status === "requestFailed" || (check?.status === "finished" && !check.result.success);
  return (
    <>
      {route && (
        <BindingEndpoint
          route={route}
          review={(!ready && !canTest) || testFailed}
          disabled={busy || manager.busy || conflict}
          onReview={() => manage(route.destination.id)}
        />
      )}
      {(error || manager.error) && (
        <p role="alert" className="text-sm text-destructive">
          {t("settings:sshManager.binding.failed")}
        </p>
      )}
      {(!route || canTest) && (
        <BindingCheckStatus check={check} imported={Boolean(route && !ready)} />
      )}
    </>
  );
}

function BindingEndpoint({
  route,
  review,
  disabled,
  onReview,
}: {
  route: SSHRoute;
  review: boolean;
  disabled: boolean;
  onReview: () => void;
}) {
  const { t } = useTranslation();
  const endpoint = connectionEndpoint(route.destination);
  return (
    <div className="space-y-2">
      {endpoint && <p className="break-all text-sm">{endpoint}</p>}
      {review && (
        <Button
          type="button"
          variant="outline"
          className={settingsActionClassName("max-md:w-full")}
          disabled={disabled}
          onClick={onReview}
        >
          {t("settings:sshManager.binding.review")}
        </Button>
      )}
    </div>
  );
}

function canTestSavedImport(
  route: SSHRoute | undefined,
  initial: ReturnType<typeof initialBinding>,
): boolean {
  return Boolean(
    route &&
    route.destination.id === initial.connection &&
    route.stamp === initial.stamp &&
    canVerifyOnFirstUse(route),
  );
}

function useManagedBinding(
  props: ManagedBindingProps,
  manager: ReturnType<typeof useSSHConnections>,
) {
  const draft = useBindingDraft(props);
  const { name, selected, conflict, initial, flow } = draft;
  const [attempt, setAttempt] = useState<BindingCheck | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const route = manager.routes.find((item) => item.destination.id === selected);
  const ready = Boolean(route && route.destination.verified_route === route.stamp);
  const canTest = ready || canTestSavedImport(route, initial);
  const check = canTest ? checkForRoute(attempt, route) : null;
  const checked = successfulCheckStamp(check);
  const canSave = ready && Boolean(name.trim() && checked) && !busy && !conflict;
  const dirty = draft.changed || Boolean(route && initial.stamp !== route.stamp);
  const test = async () => {
    if (!route || !canTest || busy || conflict) return;
    const target = { connection: selected, stamp: route.stamp };
    setBusy(true);
    setError(false);
    setAttempt({ ...target, status: "pending" });
    try {
      const result = await testSSHConnection({
        name,
        ssh_connection_id: selected,
        expected_stamp: route.stamp,
      });
      setAttempt({ ...target, status: "finished", result });
    } catch {
      setAttempt({ ...target, status: "requestFailed" });
    } finally {
      setBusy(false);
    }
  };
  const save = async () => {
    if (!route || !canSave) return;
    setBusy(true);
    setError(false);
    try {
      await props.onSave({
        name,
        identity_source: "agent",
        connection_id: selected,
        connection_stamp: route.stamp,
      });
      flow.clear();
    } catch (error) {
      setError(true);
      throw error;
    } finally {
      setBusy(false);
    }
  };
  const manage = useManageConnections(
    props,
    manager.busy || busy,
    draft.snapshot,
    draft.setSuspended,
    setError,
  );
  return {
    ...draft,
    checked,
    check,
    clearCheck: () => setAttempt(null),
    busy,
    error,
    route,
    ready,
    canTest,
    canSave,
    dirty,
    manage,
    test,
    save,
    discard: () => {
      draft.discard();
      setAttempt(null);
    },
  };
}

type BindingCheck = { connection: string; stamp: string } & (
  | { status: "pending" }
  | { status: "requestFailed" }
  | { status: "finished"; result: SSHTestResult }
);

function checkForRoute(attempt: BindingCheck | null, route: SSHRoute | undefined) {
  return attempt?.stamp === route?.stamp && attempt?.connection === route?.destination.id
    ? attempt
    : null;
}

function successfulCheckStamp(check: BindingCheck | null) {
  return check?.status === "finished" && check.result.success ? check.stamp : "";
}

function bindingCheckFailure(result: SSHTestResult) {
  const failedStep = result.steps?.find((step) => !step.success);
  switch (failedStep?.name) {
    case "Probe remote":
      return "settings:sshManager.binding.testProbeFailed";
    case "Verify platform":
      return "settings:sshManager.binding.testPlatformFailed";
    default:
      return "settings:sshManager.binding.testConnectionFailed";
  }
}

function BindingCheckStatus({
  check,
  imported,
}: {
  check: BindingCheck | null;
  imported?: boolean;
}) {
  const { t } = useTranslation();
  if (!check) {
    return (
      <p className="text-sm text-muted-foreground">
        {t(
          imported
            ? "settings:sshManager.binding.imported"
            : "settings:sshManager.binding.notChecked",
        )}
      </p>
    );
  }
  if (check.status === "pending") {
    return <AccessStatus pending title={t("settings:sshManager.binding.checking")} />;
  }
  if (check.status === "requestFailed" || !check.result.success) {
    const failure =
      check.status === "requestFailed"
        ? "settings:sshManager.binding.testRequestFailed"
        : bindingCheckFailure(check.result);
    return (
      <AccessStatus tone="error" title={t("settings:sshManager.binding.testFailed")}>
        {t(failure)}
      </AccessStatus>
    );
  }
  const { git_version, agentctl_action } = check.result;
  return (
    <AccessStatus tone="success" title={t("settings:sshManager.binding.executorReady")}>
      {!git_version && <p>{t("settings:sshManager.binding.testGitUnconfirmed")}</p>}
      {agentctl_action === "skipped" && (
        <p>{t("settings:sshManager.binding.testHelperUnconfirmed")}</p>
      )}
    </AccessStatus>
  );
}

function BindingActions({
  busy,
  canTest,
  canSave,
  coordinated,
  test,
  save,
}: {
  busy: boolean;
  canTest: boolean;
  canSave: boolean;
  coordinated: boolean;
  test: () => Promise<void>;
  save: () => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 md:flex-row md:flex-wrap">
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName()}
        disabled={!canTest || busy}
        onClick={() => {
          void test();
        }}
      >
        {t("settings:sshManager.binding.test")}
      </Button>
      {!coordinated && (
        <Button
          type="button"
          className={settingsActionClassName()}
          disabled={!canSave}
          onClick={() => {
            void save().catch(() => undefined);
          }}
        >
          {t("common:save")}
        </Button>
      )}
    </div>
  );
}

function initialBinding(props: ManagedBindingProps) {
  return {
    name: props.initial?.name ?? "",
    connection: props.initial?.connection_id ?? "",
    stamp: props.initial?.connection_stamp ?? "",
  };
}

function useManageConnections(
  props: ManagedBindingProps,
  busy: boolean,
  draft: SSHSetupDraft,
  setSuspended: (value: boolean) => void,
  setError: (value: boolean) => void,
) {
  const flow = useSSHSetupContinuation();
  const router = useRouter();
  const coordinator = useSettingsSaveCoordinator();
  return (connectionId?: string) => {
    if (busy || coordinator.exclusiveBusy || coordinator.status === "saving") return;
    if (
      coordinator.contributorStates.some(
        (item) => item.isDirty && item.id !== props.coordinatedSaveId,
      )
    ) {
      setError(true);
      return;
    }
    flushSync(() => {
      flow.begin(draft);
      setSuspended(true);
    });
    let navigated = false;
    const href = connectionId
      ? sshConnectionSettingsPath(undefined, { id: connectionId, section: "access" })
      : sshConnectionSettingsPath();
    router.push(href, {
      onNavigated: () => {
        navigated = true;
      },
    });
    if (!navigated) {
      coordinator.cancelPendingNavigation();
      flow.clear();
      setSuspended(false);
      setError(true);
    }
  };
}

function DraftConflict({ onDiscard }: { onDiscard: () => void }) {
  const { t } = useTranslation();
  return (
    <div role="alert" className="space-y-2 text-sm text-destructive">
      <p>{t("settings:sshManager.binding.draftConflict")}</p>
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName()}
        onClick={onDiscard}
      >
        {t("settings:sshManager.binding.reloadDraft")}
      </Button>
    </div>
  );
}

function BindingPicker({
  selected,
  manager,
  busy,
  onChange,
}: {
  selected: string;
  manager: ReturnType<typeof useSSHConnections>;
  busy: boolean;
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <ConnectionPicker
      id="managed-executor-connection"
      label={t("settings:sshManager.binding.connection")}
      value={selected}
      disabled={busy || manager.busy}
      onChange={onChange}
      options={[
        { value: "", label: t("settings:sshManager.selectConnection") },
        ...manager.routes
          .filter(
            (item) =>
              item.destination.verified_route === item.stamp || item.destination.id === selected,
          )
          .map((item) => ({ value: item.destination.id, label: item.destination.name })),
      ]}
    />
  );
}

function useBindingDraft(props: ManagedBindingProps) {
  const initial = initialBinding(props);
  const flow = useSSHSetupContinuation();
  const editor = usePathname();
  const restored = flow.draft?.editor === editor ? flow.draft : null;
  const baseline = JSON.stringify(initial);
  const conflict = Boolean(restored && restored.baseline !== baseline);
  const [name, setName] = useState(restored?.name ?? initial.name);
  const [selected, setSelected] = useState(restored?.connection ?? initial.connection);
  const [suspended, setSuspended] = useState(false);
  useEffect(() => {
    if (restored) document.getElementById("managed-executor-connection")?.focus();
  }, [restored]);
  return {
    initial,
    flow,
    name,
    setName,
    selected,
    setSelected,
    conflict,
    suspended,
    setSuspended,
    changed: name !== initial.name || selected !== initial.connection,
    snapshot: {
      editor,
      name,
      connection: selected,
      baseline,
      ownerUserId: props.ownerUserId ?? "",
    },
    discard: () => {
      flow.clear();
      setName(initial.name);
      setSelected(initial.connection);
    },
  };
}
