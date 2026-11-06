"use client";

import {
  IconCheck,
  IconClock,
  IconPlayerPlay,
  IconAlertCircle,
  IconRefresh,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { MCPSessionSelectionState } from "@/lib/types/http-mcp";
import { cn } from "@/lib/utils";
import { mcpSelectionPresentation } from "./mcp-selection-presentation";

const STATE_STYLES = {
  applied: { icon: IconCheck, accent: "text-emerald-700 dark:text-emerald-400", surface: "" },
  pending_idle: {
    icon: IconClock,
    accent: "text-amber-700 dark:text-amber-400",
    surface: "border-amber-500/20 bg-amber-500/5",
  },
  deferred_restart: {
    icon: IconPlayerPlay,
    accent: "text-blue-700 dark:text-blue-400",
    surface: "border-blue-500/20 bg-blue-500/5",
  },
  failed: {
    icon: IconAlertCircle,
    accent: "text-destructive",
    surface: "border-destructive/20 bg-destructive/5",
  },
};

export function McpSelectionNotice({
  state,
  retry,
  retrying,
  compact = false,
  changes,
}: {
  state: MCPSessionSelectionState;
  retry?: () => void;
  retrying?: boolean;
  compact?: boolean;
  changes?: ReactNode;
}) {
  const { t } = useTranslation();
  const presentation = mcpSelectionPresentation(state);
  const style = STATE_STYLES[presentation.tone];
  const Icon = style.icon;
  return (
    <div
      data-testid="mcp-application-summary"
      data-state={state.apply_state}
      role="status"
      className={cn(
        "flex min-w-0 items-start gap-2.5",
        !compact && "rounded-lg p-3",
        !compact && state.apply_state !== "applied" && "border",
        !compact && style.surface,
      )}
    >
      <Icon className={cn("mt-0.5 size-4 shrink-0", style.accent)} aria-hidden="true" />
      <div className="min-w-0 flex-1 space-y-1">
        <p
          className={cn("text-[13px] font-medium", state.apply_state !== "applied" && style.accent)}
        >
          {t(presentation.labelKey)}
        </p>
        <p className="max-w-prose break-words text-pretty text-xs text-muted-foreground">
          {t(presentation.descriptionKey)}
        </p>
        {changes}
        {state.apply_state === "failed" && retry && (
          <Button
            type="button"
            variant="outline"
            className="mt-2 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={retry}
            disabled={retrying}
          >
            <IconRefresh className="mr-2 size-4" aria-hidden="true" />
            {t("settings:mcpSessionRetry")}
          </Button>
        )}
      </div>
    </div>
  );
}
