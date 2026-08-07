"use client";

import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { ConnectionIssueSeverity, ConnectionStatus } from "@/lib/types/connection";

/**
 * Copy is returned as catalog keys, not resolved strings: these are plain
 * functions (no hook available) and every consumer is a component, so resolving
 * at render keeps a locale switch working. Resolving here with the module-level
 * `t` would work today but reads as module-scope copy the next reader must
 * re-verify.
 */
export type ConnectionStatusDetails = {
  labelKey: string;
  descriptionKey: string;
  descriptionValues?: Record<string, string>;
  dotClass: string;
  animate: boolean;
};

export type ConnectionIssueDetails = {
  labelKey: string;
  descriptionKey: string;
  /** Always absent for issue details; present so the two shapes stay unifiable. */
  descriptionValues?: Record<string, string>;
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
        labelKey: "sidebar:connected",
        descriptionKey: "sidebar:connectedToKandev",
        dotClass: "bg-success",
        animate: false,
      };
    case "connecting":
      return {
        labelKey: "sidebar:connecting",
        descriptionKey: "sidebar:connectingToKandev",
        dotClass: "bg-muted-foreground",
        animate: true,
      };
    case "reconnecting":
      return {
        labelKey: "sidebar:reconnecting",
        descriptionKey: "sidebar:reconnectingToKandev",
        dotClass: "bg-amber-500",
        animate: true,
      };
    case "error":
      return {
        labelKey: "sidebar:connectionError",
        // The interpolated `error` is a transport/API diagnostic and stays
        // English by design — see docs/i18n.md on interpolated values.
        descriptionKey: error ? "sidebar:connectionErrorDetail" : "sidebar:connectionError",
        descriptionValues: error ? { error } : undefined,
        dotClass: "bg-destructive",
        animate: false,
      };
    case "disconnected":
      return {
        labelKey: "sidebar:offline",
        descriptionKey: "sidebar:connectionUnavailable",
        dotClass: "bg-muted-foreground/50",
        animate: false,
      };
  }
}

export function connectionIssueDetails(
  severity: Exclude<ConnectionIssueSeverity, "none">,
): ConnectionIssueDetails {
  switch (severity) {
    case "unstable":
      return {
        labelKey: "sidebar:connectionUnstable",
        descriptionKey: "sidebar:connectionUnstableDescription",
        dotClass: "bg-amber-500",
        animate: false,
      };
    case "lost":
      return {
        labelKey: "sidebar:connectionLost",
        descriptionKey: "sidebar:connectionLostDescription",
        dotClass: "bg-destructive",
        animate: false,
      };
  }
}

/**
 * Resolved connection-issue copy for the surfaces that only need the sentence
 * (mobile header, menu sheet, bottom nav, sidebar fallback). Keeps `t` out of
 * four unrelated components and returns null while the connection is healthy.
 */
export function useConnectionIssueCopy(severity: ConnectionIssueSeverity) {
  const { t } = useTranslation();
  if (severity === "none") return null;
  const details = connectionIssueDetails(severity);
  return {
    ...details,
    label: t(details.labelKey),
    description: t(details.descriptionKey),
  };
}

export function ConnectionStatusItem({ presentation }: { presentation: "bar" | "mobile-drawer" }) {
  const { t } = useTranslation();
  const status = useAppStore((state) => state.connection.status);
  const error = useAppStore((state) => state.connection.error);
  const severity = useAppStore((state) => state.connection.issueSeverity);
  if (severity === "none" && presentation === "bar") return null;
  const issueActive = severity !== "none";
  const details = issueActive
    ? connectionIssueDetails(severity)
    : connectionStatusDetails(status, error);
  const description = t(details.descriptionKey, { ...details.descriptionValues });

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center leading-none",
            presentation === "bar" ? "h-full w-5 justify-center" : "min-h-11 gap-3 px-1 text-sm",
          )}
          role="status"
          aria-label={description}
          tabIndex={presentation === "bar" ? 0 : undefined}
          data-testid="app-status-connection"
          data-connection-severity={issueActive ? severity : undefined}
        >
          <span
            className={`size-1.5 shrink-0 rounded-full ${details.dotClass} ${issueActive || details.animate ? "animate-pulse" : ""}`}
            aria-hidden="true"
          />
          {presentation === "bar" ? (
            <span className="sr-only">{t(details.labelKey)}</span>
          ) : (
            <span className="flex min-w-0 flex-col gap-0.5">
              <span className="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
                {t("sidebar:connection")}
              </span>
              <span className="text-sm font-medium text-foreground">{description}</span>
            </span>
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent>{description}</TooltipContent>
    </Tooltip>
  );
}
