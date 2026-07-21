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
        description: "Connected to Kandev",
        dotClass: "bg-success",
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

export function ConnectionStatusItem({ presentation }: { presentation: "bar" | "mobile-drawer" }) {
  const status = useAppStore((state) => state.connection.status);
  const error = useAppStore((state) => state.connection.error);
  const details = connectionStatusDetails(status, error);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex h-full items-center leading-none",
            presentation === "bar" ? "w-5 justify-center" : "min-h-11 gap-3 px-1 text-sm",
          )}
          role="status"
          aria-label={details.description}
          data-testid="app-status-connection"
        >
          <span
            className={`size-1.5 shrink-0 rounded-full ${details.dotClass} ${details.animate ? "animate-pulse" : ""}`}
            aria-hidden="true"
          />
          <span className={presentation === "bar" ? "sr-only" : "text-foreground"}>
            {presentation === "bar" ? details.label : details.description}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{details.description}</TooltipContent>
    </Tooltip>
  );
}
