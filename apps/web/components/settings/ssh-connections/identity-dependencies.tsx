"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useSearchParams } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import { sshConnectionSettingsPath } from "@/lib/settings/ssh-connection-settings-routes";
import type { SSHIdentityDependency } from "@/lib/types/http-ssh-connections";
import { settingsActionClassName } from "../settings-control";

export function IdentityDependencies({ dependencies }: { dependencies: SSHIdentityDependency[] }) {
  const { t } = useTranslation();
  const query = useSearchParams();
  if (!dependencies.length) return null;
  return (
    <section className="space-y-2 min-w-0" aria-label={t("settings:sshManager.dependencies.title")}>
      <p className="text-sm text-muted-foreground">{t("settings:sshManager.dependencies.help")}</p>
      <ul className="space-y-1 min-w-0">
        {dependencies.map((dependency) => {
          const label = t(`settings:sshManager.dependencies.${dependency.kind}`, {
            name: dependency.label || dependency.id,
          });
          let href: string | null = null;
          if (dependency.kind === "connection") {
            href = sshConnectionSettingsPath(new URLSearchParams(query.toString()), {
              tab: "connections",
              id: dependency.id,
              new: null,
            });
          } else if (dependency.kind === "executor") {
            href = `/settings/executors/ssh/${encodeURIComponent(dependency.id)}`;
          } else if (dependency.task_id) {
            href = linkToTask(dependency.task_id, { sessionId: dependency.id });
          }
          return (
            <li key={`${dependency.kind}:${dependency.id}`} className="min-w-0">
              {href ? (
                <Button
                  asChild
                  variant="link"
                  className={settingsActionClassName(
                    "h-auto max-w-full justify-start whitespace-normal text-left px-0",
                  )}
                >
                  <Link href={href} className="cursor-pointer break-all">
                    {label}
                  </Link>
                </Button>
              ) : (
                <p className="text-sm break-all">{label}</p>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
