"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { usePathname } from "@/lib/routing/client-router";
import { useAgentRuntimeUpdateStatuses } from "@/hooks/domains/settings/use-agent-runtime-update-statuses";
import { useUpdateAvailableToast } from "@/hooks/use-update-available-toast";

/** Shares runtime discovery with Settings and keeps an app-wide update entry. */
export function UpdateAvailableToastBridge() {
  useUpdateAvailableToast();
  const jobs = useAppStore((s) => s.updateJobs.byAgent);
  const notification = useAppStore((s) => s.updateAvailableNotification);
  const { statusByAgent, refresh } = useAgentRuntimeUpdateStatuses(jobs);
  const pathname = usePathname();
  const { t } = useTranslation();
  useEffect(() => {
    const timer = setInterval(() => void refresh(), 60_000);
    return () => clearInterval(timer);
  }, [refresh]);
  useEffect(() => {
    if (notification?.agent_name) void refresh();
  }, [notification, refresh]);
  const count = Object.values(statusByAgent).filter(
    (s) => s.available && s.enabled && s.check_state === "update_available",
  ).length;
  if (!count || pathname.startsWith("/settings")) return null;
  return (
    <Button
      asChild
      variant="outline"
      className="fixed right-6 bottom-[calc(5.25rem+var(--app-status-bar-height)+env(safe-area-inset-bottom,0px))] z-[41] h-11 min-h-11 shadow-md md:right-24 md:bottom-[calc(2.125rem+var(--app-status-bar-height))] md:h-7 md:min-h-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid="agent-runtime-update-indicator"
    >
      <Link href="/settings/agents#runtime-updates">
        <IconDownload className="mr-2 size-4" />
        {t("agents:runtimeIndicator", { count })}
      </Link>
    </Button>
  );
}
