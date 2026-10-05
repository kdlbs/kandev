"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useAgentRuntimeUpdateStatuses } from "@/hooks/domains/settings/use-agent-runtime-update-statuses";
import { useUpdateAvailableToast } from "@/hooks/use-update-available-toast";

/** Shares runtime discovery with Settings and consumes update notifications. */
export function UpdateAvailableToastBridge() {
  useUpdateAvailableToast();
  const jobs = useAppStore((s) => s.updateJobs.byAgent);
  const notification = useAppStore((s) => s.updateAvailableNotification);
  const { refresh } = useAgentRuntimeUpdateStatuses(jobs);
  useEffect(() => {
    const timer = setInterval(() => void refresh(), 60_000);
    return () => clearInterval(timer);
  }, [refresh]);
  useEffect(() => {
    if (notification?.agent_name || notification?.notification_kind === "agent_runtime_summary") {
      void refresh();
    }
  }, [notification, refresh]);
  return null;
}
