"use client";
import { useTranslation } from "react-i18next";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { ManagerNavigation } from "./use-manager-navigation";
import { IdentityForm } from "./identity-form";
import { IdentityDetails } from "./identity-details";
import { ConnectionDetails } from "./connection-details";
import { ConnectionOverview } from "./connection-overview";
import { ConnectionAccess } from "./connection-access";
import { PrerequisitePanel } from "./prerequisite-panel";

export function ManagerDetail({
  manager,
  navigation: n,
}: {
  manager: SSHManager;
  navigation: ManagerNavigation;
}) {
  const { t } = useTranslation();
  if (n.identities) {
    if (n.creating)
      return (
        <IdentityForm
          manager={manager}
          onCreated={(identity) => n.identityCreated(identity.id)}
          onCancel={n.back}
        />
      );
    const identity = manager.identities.find((item) => item.id === n.selected);
    if (identity)
      return (
        <IdentityDetails
          key={identity.id}
          identity={identity}
          manager={manager}
          onDeleted={n.back}
          editing={n.editing}
          onSaved={n.cancelEdit}
        />
      );
  } else {
    const route = manager.routes.find((item) => item.destination.id === n.selected);
    if (n.creating || (route && n.editing))
      return (
        <ConnectionDetails
          key={route?.destination.id ?? "new"}
          route={n.creating ? undefined : route}
          manager={manager}
          onSaved={(saved) =>
            n.saved(saved.destination.id, saved.destination.verified_route === saved.stamp)
          }
          onDeleted={n.back}
          onCreateBastion={(saved) => n.createBastion(saved.destination.id)}
          onCreateIdentity={(saved) => n.createIdentity(saved.destination.id)}
        />
      );
    if (route) {
      if (n.section === "access")
        return (
          <ConnectionAccess key={route.stamp} route={route} manager={manager} onEdit={n.edit} />
        );
      if (n.section === "diagnostics")
        return <PrerequisitePanel key={route.stamp} route={route} manager={manager} />;
      return (
        <ConnectionOverview
          route={route}
          manager={manager}
          onAccess={() => n.setSection("access")}
        />
      );
    }
  }
  return (
    <p role="alert" className="text-sm text-muted-foreground">
      {t("settings:sshManager.recordUnavailable")}
    </p>
  );
}
