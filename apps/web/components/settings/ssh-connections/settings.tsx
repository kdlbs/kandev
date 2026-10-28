"use client";
import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useSSHConnections, type SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { SettingsTabs, SettingsTabsPanel } from "../settings-tabs";
import { settingsActionClassName } from "../settings-control";
import { useManagerNavigation, type ManagerNavigation } from "./use-manager-navigation";
import { SetupReturnActions } from "./setup-return-actions";
import { AdoptionFinish } from "./adoption";
import { ManagerDetail } from "./manager-detail";
import { ConnectionCollection } from "./connection-collection";
import { ManagerHeader } from "./manager-header";

export function SSHConnectionsSettings() {
  const enabled = useFeature("sshConnectionManager");
  const admin = useIsAdmin();
  const { t } = useTranslation();
  if (!enabled || !admin) return <p>{t("settings:sshManager.unavailable")}</p>;
  return <SSHConnectionsContent />;
}
function SSHConnectionsContent() {
  const manager = useSSHConnections();
  const n = useManagerNavigation();
  const { t } = useTranslation();
  const details = n.creating || Boolean(n.selected);
  const detailTabs = details && !n.identities;
  const tabs = detailTabs
    ? ["overview", "access", "diagnostics"].map((id) => ({
        id,
        label: t(`settings:sshManager.ui.${id}`),
      }))
    : [
        { id: "connections", label: t("settings:sshManager.connections") },
        { id: "identities", label: t("settings:sshManager.identities") },
      ];
  let value = n.identities ? "identities" : "connections";
  if (detailTabs) value = n.section;
  return (
    <SettingsTabs
      tabs={tabs}
      value={value}
      onValueChange={(next) => {
        if (details && !n.identities) n.setSection(next);
        else if (next === "identities") n.showIdentities();
        else n.showConnections();
      }}
    >
      <div className="space-y-8 min-w-0 pb-[max(2rem,env(safe-area-inset-bottom))]">
        {details && (
          <Button
            variant="ghost"
            className={settingsActionClassName("-ml-2 text-muted-foreground")}
            onClick={n.back}
          >
            <IconArrowLeft className="size-3.5 mr-1.5" aria-hidden />
            {t(
              n.identities
                ? "settings:sshManager.ui.backIdentities"
                : "settings:sshManager.ui.backConnections",
            )}
          </Button>
        )}
        <ManagerHeader manager={manager} navigation={n} />
        <ManagerContext manager={manager} navigation={n} />
        <ManagerError manager={manager} />
        {manager.busy && (
          <p role="status" className="text-sm text-muted-foreground">
            {t("settings:sshManager.ui.working")}
          </p>
        )}
        {!manager.loaded && !manager.error && (
          <p role="status" className="text-sm text-muted-foreground">
            {t("settings:sshManager.loading")}
          </p>
        )}
        <SettingsTabsPanel key={value} value={value}>
          {manager.loaded &&
            (details ? (
              <ManagerDetail manager={manager} navigation={n} />
            ) : (
              <ConnectionCollection manager={manager} navigation={n} />
            ))}
        </SettingsTabsPanel>
      </div>
    </SettingsTabs>
  );
}

function ManagerContext({
  manager,
  navigation: n,
}: {
  manager: SSHManager;
  navigation: ManagerNavigation;
}) {
  const { t } = useTranslation();
  const route = manager.routes.find((item) => item.destination.id === n.selected);
  return (
    <>
      <SetupReturnActions
        route={
          !n.identities && !n.creating && !n.returnConnection && !n.editing ? route : undefined
        }
        busy={manager.busy}
      />
      {n.returnConnection && (
        <Button
          variant="outline"
          className={settingsActionClassName()}
          onClick={n.returnToDestination}
        >
          {t("settings:sshManager.returnToDestination")}
        </Button>
      )}
      {!n.creating &&
        !n.editing &&
        route &&
        route.destination.id === n.adoptConnection &&
        n.adoptExecutor &&
        n.adoptDigest && (
          <AdoptionFinish executorId={n.adoptExecutor} digest={n.adoptDigest} route={route} />
        )}
    </>
  );
}

function ManagerError({ manager }: { manager: SSHManager }) {
  const { t } = useTranslation();
  if (!manager.error) return null;
  return (
    <div
      role="alert"
      className="rounded-md border border-destructive/40 bg-destructive/5 p-4 space-y-3 text-sm"
    >
      <p>
        {t(`settings:sshManager.errors.${manager.error}`, {
          defaultValue: t("settings:sshManager.errors.ssh_operation_failed"),
        })}
      </p>
      <Button
        variant="outline"
        className={settingsActionClassName()}
        disabled={manager.busy}
        onClick={() => {
          void manager.reload().catch(() => undefined);
        }}
      >
        {t("settings:sshManager.reload")}
      </Button>
    </div>
  );
}
