"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { ConnectionStatus } from "@/lib/types/connection";

export type ConnectionStatusDetails = {
  label: string;
  description: string;
  dotClass: string;
  animate: boolean;
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
        dotClass: "bg-emerald-500",
        animate: false,
      };
    case "connecting":
      return {
        label: "Connecting",
        description: "Connecting to Kandev",
        dotClass: "bg-muted-foreground",
        animate: true,
      };
    case "reconnecting":
      return {
        label: "Reconnecting",
        description: "Reconnecting to Kandev",
        dotClass: "bg-amber-500",
        animate: true,
      };
    case "error":
      return {
        label: "Connection error",
        description: error ? `Connection error: ${error}` : "Connection error",
        dotClass: "bg-destructive",
        animate: false,
      };
    case "disconnected":
      return {
        label: "Offline",
        description: "Connection unavailable",
        dotClass: "bg-muted-foreground/50",
        animate: false,
      };
  }
}

export function ConnectionStatusItem({ className }: { className?: string }) {
  const status = useAppStore((state) => state.connection.status);
  const error = useAppStore((state) => state.connection.error);
  const details = connectionStatusDetails(status, error);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex w-6 items-center justify-center leading-none",
            className ?? "h-6",
          )}
          role="status"
          aria-label={details.description}
          data-testid="app-status-connection"
        >
          <span
            className={`h-2 w-2 rounded-full ${details.dotClass} ${details.animate ? "animate-pulse" : ""}`}
            aria-hidden="true"
          />
          <span className="sr-only">{details.label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{details.description}</TooltipContent>
    </Tooltip>
  );
}
