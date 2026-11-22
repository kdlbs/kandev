"use client";
import { useTranslation } from "react-i18next";
import { IconPlus, IconPencil } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { ConnectionStatusBadge } from "./status-badge";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { ManagerNavigation } from "./use-manager-navigation";
import { SettingsPageHeader } from "../settings-typography";
import { SettingsTabsList } from "../settings-tabs";
import { settingsActionClassName } from "../settings-control";
import { ResourceActions } from "./resource-actions";
import { connectionState } from "./connection-presentation";

export function ManagerHeader({
  manager,
  navigation: n,
}: {
  manager: SSHManager;
  navigation: ManagerNavigation;
}) {
  const { t } = useTranslation();
  const details = n.creating || Boolean(n.selected);
  const { name, description } = useManagerTitle(manager, n);
  const showTabs = !n.creating && !n.editing && (!details || !n.identities);
  return (
    <div className="space-y-4 min-w-0">
      <SettingsPageHeader
        title={<span className="break-words">{name}</span>}
        description={
          <span className="flex flex-wrap items-center gap-2 [overflow-wrap:anywhere]">
            <ManagerStatus manager={manager} navigation={n} />
            <span>{description}</span>
          </span>
        }
        actions={
          details ? (
            <ManagerActions manager={manager} navigation={n} details={details} />
          ) : undefined
        }
      />
      {showTabs && (
        <div className="flex min-w-0 flex-col gap-3 md:flex-row md:flex-wrap md:items-center">
          <SettingsTabsList
            ariaLabel={t("settings:sshManager.title")}
            className="w-full md:w-fit"
          />
          {!details && <ManagerActions manager={manager} navigation={n} details={false} />}
        </div>
      )}
    </div>
  );
}

function useManagerTitle(manager: SSHManager, n: ManagerNavigation) {
  const { t } = useTranslation();
  const details = n.creating || Boolean(n.selected);
  const route = manager.routes.find((item) => item.destination.id === n.selected);
  const identity = manager.identities.find((item) => item.id === n.selected);
  let name = t("settings:sshManager.title");
  let description: string | undefined = t("settings:sshManager.description");
  if (details) {
    name =
      (n.identities ? identity?.label : route?.destination.name) ??
      t("settings:sshManager.recordUnavailable");
    description = n.identities ? identity?.fingerprint : route?.destination.host;
  }
  if (n.creating) {
    description = undefined;
    name = t(
      n.identities ? "settings:sshManager.newIdentity" : "settings:sshManager.newConnection",
    );
  }
  return { name, description };
}
function ManagerStatus({
  manager,
  navigation: n,
}: {
  manager: SSHManager;
  navigation: ManagerNavigation;
}) {
  const route = manager.routes.find((item) => item.destination.id === n.selected);
  if (!route || n.creating || n.editing) return null;
  return (
    <ConnectionStatusBadge
      className="text-xs"
      state={connectionState(
        route,
        manager.identities,
        manager.failedTests?.[route.destination.id],
      )}
    />
  );
}

export function ManagerActions({
  manager,
  navigation: n,
  details,
}: {
  manager: SSHManager;
  navigation: ManagerNavigation;
  details: boolean;
}) {
  const { t } = useTranslation();
  if (!details)
    return (
      <Button
        id="ssh-create-resource"
        className={settingsActionClassName("max-md:w-full")}
        disabled={manager.busy}
        onClick={n.create}
      >
        <IconPlus className="size-3.5 mr-1.5" aria-hidden />
        {t(n.identities ? "settings:sshManager.newIdentity" : "settings:sshManager.newConnection")}
      </Button>
    );
  if (n.creating) return null;
  if (n.editing)
    return (
      <Button variant="outline" className={settingsActionClassName()} onClick={n.cancelEdit}>
        {t("common:cancel")}
      </Button>
    );
  const route = manager.routes.find((item) => item.destination.id === n.selected);
  const identity = manager.identities.find((item) => item.id === n.selected);
  if (n.identities ? !identity : !route) return null;
  const label = t(
    n.identities ? "settings:sshManager.deleteIdentity" : "settings:sshManager.deleteConnection",
  );
  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        className={settingsActionClassName()}
        onClick={n.edit}
        disabled={manager.busy}
      >
        <IconPencil className="size-3.5 mr-1.5" aria-hidden />
        {t("common:edit")}
      </Button>
      <ResourceActions
        key={n.selected}
        label={label}
        description={t(
          n.identities
            ? "settings:sshManager.deleteIdentityWarning"
            : "settings:sshManager.deleteConnectionWarning",
        )}
        target={n.selected!}
        busy={manager.busy || Boolean(n.identities && identity?.dependencies?.length)}
        onDelete={async () => {
          if (n.identities && identity) await manager.deleteIdentity(identity);
          else if (route) await manager.deleteConnection(route);
          n.back();
        }}
      />
    </div>
  );
}
