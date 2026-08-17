"use client";

import { useState } from "react";
import { IconAlertTriangle, IconChevronDown, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import type { RunError } from "@/app/office/tasks/[id]/types";
import { formatRelativeTime } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export function ManagedRuntimeNpmRunError({
  error,
  agentName,
  onRetry,
}: {
  error: RunError;
  agentName: string;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const [showDetails, setShowDetails] = useState(false);
  const technicalDetails = error.failureDetails;

  return (
    <div
      className="flex gap-3 border-b border-border/50 py-3"
      data-testid="run-error-managed-runtime-npm-recovery"
      role="alert"
    >
      <AgentAvatar name={agentName} size="md" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">{agentName}</span>
          <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
            <IconAlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
            {t("chat:managedRuntimeNpmTitle")}
          </span>
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(error.failedAt)}
          </span>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">{t("chat:managedRuntimeNpmBody")}</p>
        {technicalDetails && (
          <Collapsible open={showDetails} onOpenChange={setShowDetails} className="mt-2">
            <CollapsibleTrigger className="flex min-h-11 cursor-pointer items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground sm:min-h-8">
              <IconChevronDown
                className={`h-3.5 w-3.5 transition-transform ${showDetails ? "rotate-180" : ""}`}
              />
              {t("chat:technicalDetails")}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <pre className="mt-1 max-h-[300px] max-w-full overflow-auto whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px] text-muted-foreground">
                {technicalDetails}
              </pre>
            </CollapsibleContent>
          </Collapsible>
        )}
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-auto min-h-11 cursor-pointer gap-1.5 text-xs sm:min-h-8"
            onClick={onRetry}
            data-testid="run-error-managed-runtime-retry-button"
          >
            <IconRefresh className="h-3 w-3" />
            {t("chat:managedRuntimeRetry")}
          </Button>
        </div>
      </div>
    </div>
  );
}
