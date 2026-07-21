"use client";

import {
  IconAlertCircle,
  IconCircleCheck,
  IconLoader2,
  IconPlugConnectedX,
  IconRefresh,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { ConnectionStatus } from "@/lib/types/connection";

export type ConnectionStatusDetails = {
  label: string;
  description: string;
  className: string;
  Icon: typeof IconCircleCheck;
  animate?: boolean;
};

export function connectionStatusDetails(
  status: ConnectionStatus,
  error: string | null,
): ConnectionStatusDetails {
  switch (status) {
    case "connected":
      return {
        label: "Connected",
        description: "Connection active",
        className: "text-emerald-600 dark:text-emerald-400",
        Icon: IconCircleCheck,
      };
    case "connecting":
      return {
        label: "Connecting",
        description: "Connecting to Kandev",
        className: "text-muted-foreground",
        Icon: IconLoader2,
        animate: true,
      };
    case "reconnecting":
      return {
        label: "Reconnecting",
        description: "Reconnecting to Kandev",
        className: "text-amber-600 dark:text-amber-400",
        Icon: IconRefresh,
        animate: true,
      };
    case "error":
      return {
        label: "Connection error",
        description: error ? `Connection error: ${error}` : "Connection error",
        className: "text-destructive",
        Icon: IconAlertCircle,
      };
    case "disconnected":
      return {
        label: "Offline",
        description: "Connection unavailable",
        className: "text-muted-foreground",
        Icon: IconPlugConnectedX,
      };
  }
}

export function ConnectionStatusItem({ compact = false }: { compact?: boolean }) {
  const status = useAppStore((state) => state.connection.status);
  const error = useAppStore((state) => state.connection.error);
  const details = connectionStatusDetails(status, error);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`inline-flex h-6 items-center gap-1.5 px-2 text-xs ${details.className}`}
          role="status"
          aria-label={details.description}
          data-testid="app-status-connection"
        >
          <details.Icon className={`h-3.5 w-3.5 ${details.animate ? "animate-spin" : ""}`} />
          <span className={compact ? "sr-only" : "truncate"}>{details.label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{details.description}</TooltipContent>
    </Tooltip>
  );
}
