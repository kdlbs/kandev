"use client";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconKey, IconServer } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import Link from "@/components/routing/app-link";
import { sshConnectionSettingsPath } from "@/lib/settings/ssh-connection-settings-routes";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { ManagerNavigation } from "./use-manager-navigation";
import { connectionState, connectionDiscriminator } from "./connection-presentation";
import { SettingsGroup } from "../settings-group";

export function ConnectionCollection({
  manager,
  navigation: n,
}: {
  manager: SSHManager;
  navigation: ManagerNavigation;
}) {
  const { t } = useTranslation();
  const items = n.identities
    ? manager.identities.map((identity) => ({
        id: identity.id,
        name: identity.label,
        description: t(`settings:sshManager.ui.${identity.kind}Identity`),
        detail: [
          identity.fingerprint || identity.reference,
          t("settings:sshManager.ui.references", { count: identity.dependencies?.length ?? 0 }),
        ]
          .filter(Boolean)
          .join(" · "),
        state: null,
      }))
    : manager.routes.map((route) => ({
        id: route.destination.id,
        name: route.destination.name,
        description: `${route.destination.user}@${route.destination.host}:${route.destination.port}`,
        detail: [
          manager.identities.find((i) => i.id === route.destination.identity_id)?.label,
          route.bastion?.name,
          connectionDiscriminator(route, manager.routes),
        ]
          .filter(Boolean)
          .join(" · "),
        state: connectionState(
          route,
          manager.identities,
          manager.failedTests?.[route.destination.id],
        ),
      }));
  if (!items.length)
    return (
      <p className="py-8 text-sm text-muted-foreground">
        {t(
          n.identities
            ? "settings:sshManager.emptyIdentities"
            : "settings:sshManager.emptyConnections",
        )}
      </p>
    );
  const Icon = n.identities ? IconKey : IconServer;
  return (
    <SettingsGroup
      title={t(n.identities ? "settings:sshManager.identities" : "settings:sshManager.connections")}
      contentClassName="px-0 py-0 overflow-hidden"
    >
      <ul className="divide-y divide-border/70">
        {items.map((item) => (
          <li key={item.id}>
            <Link
              id={`ssh-row-${item.id}`}
              href={sshConnectionSettingsPath(new URLSearchParams(window.location.search), {
                id: item.id,
                new: null,
                mode: null,
                section: null,
                select_identity: null,
              })}
              className="flex min-w-0 items-center gap-4 px-4 py-4 hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring cursor-pointer transition-colors duration-150 motion-reduce:transition-none"
            >
              <span className="rounded-md bg-muted p-2 text-muted-foreground shrink-0">
                <Icon className="size-4" aria-hidden />
              </span>
              <span className="min-w-0 flex-1 space-y-1">
                <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span className="font-medium text-sm break-words">{item.name}</span>
                  {item.state && (
                    <Badge variant="outline" className="font-normal shrink-0">
                      {t(`settings:sshManager.ui.states.${item.state}`)}
                    </Badge>
                  )}
                </span>
                <span className="block text-xs text-muted-foreground break-all">
                  {item.description}
                </span>
                {item.detail && (
                  <span className="block text-xs text-muted-foreground break-all">
                    {item.detail}
                  </span>
                )}
              </span>
              <IconChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />
            </Link>
          </li>
        ))}
      </ul>
    </SettingsGroup>
  );
}
