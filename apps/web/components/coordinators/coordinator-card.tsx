"use client";

import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

type CoordinatorCardProps = {
  coordinator: Coordinator;
  agentProfileLabel: string;
  executorProfileLabel: string;
  openHref: string;
  configureHref: string;
};

// One card per coordinator (AC-004.2): name, agent profile, executor,
// context, Open (unconditional href to Needs you, built by a later work
// order — D7) and Configure.
export function CoordinatorCard({
  coordinator,
  agentProfileLabel,
  executorProfileLabel,
  openHref,
  configureHref,
}: CoordinatorCardProps) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-2 rounded-lg border bg-card p-4"
      data-testid={`coordinator-card-${coordinator.id}`}
    >
      <Link
        href={configureHref}
        data-testid={`coordinator-name-link-${coordinator.id}`}
        className="font-medium text-primary hover:underline"
      >
        {coordinator.name}
      </Link>
      <p className="text-sm text-muted-foreground">
        {t("coordinator:cardAgentAndExecutor", {
          agent: agentProfileLabel,
          executor: executorProfileLabel,
        })}
      </p>
      {coordinator.context && (
        <p className="line-clamp-2 text-sm text-muted-foreground">{coordinator.context}</p>
      )}
      <div className="flex items-center gap-3 pt-1">
        <Button asChild variant="outline" className="cursor-pointer">
          <Link href={openHref} data-testid={`coordinator-open-${coordinator.id}`}>
            {t("coordinator:open")}
          </Link>
        </Button>
        <Link
          href={configureHref}
          data-testid={`coordinator-configure-${coordinator.id}`}
          className={controlSizingClassName(
            "standard",
            "inline-flex items-center text-sm text-primary hover:underline",
          )}
        >
          {t("coordinator:configure")}
        </Link>
      </div>
    </div>
  );
}
