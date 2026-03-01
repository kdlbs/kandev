"use client";

import { useState, useCallback } from "react";
import { useSystemHealth } from "@/hooks/domains/settings/use-system-health";

export function useSystemHealthIndicator() {
  const { issues, healthy, loaded } = useSystemHealth();
  const [dialogOpen, setDialogOpen] = useState(false);

  const hasIssues = loaded && !healthy && issues.length > 0;

  const openDialog = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const closeDialog = useCallback(() => {
    setDialogOpen(false);
  }, []);

  return {
    hasIssues,
    issues,
    dialogOpen,
    openDialog,
    closeDialog,
  };
}
