"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useBuiltinMCPSession } from "@/hooks/domains/session/use-builtin-mcp-session";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import { automationMCPEvidence } from "./automation-mcp-evidence-model";

/**
 * Load run evidence only while expanded and display desired choices separately from bound and
 * observed tools.
 */
export function AutomationMCPEvidence({
  automation,
  run,
}: {
  automation: Automation;
  run: AutomationRun;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { view } = useBuiltinMCPSession(
    run.task_id || null,
    run.session_id ?? null,
    automation.workspace_id,
    open,
    run,
  );
  const evidence = automationMCPEvidence(automation, run, view);
  const names = (packages: typeof evidence.desired) =>
    packages?.map((id) => t(`settings:builtinMcp.packages.${id}.label`)).join(", ") ||
    t("automations:mcpUnknown");
  return (
    <details
      onToggle={(event) => setOpen(event.currentTarget.open)}
      className="shrink-0 border-b px-4 py-2 text-xs"
      data-testid="automation-mcp-evidence"
    >
      <summary className="cursor-pointer py-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        {t("automations:mcpAccess")}
      </summary>
      <dl className="grid min-w-0 grid-cols-1 gap-1 pb-2 sm:grid-cols-[auto_1fr] sm:gap-x-4">
        <dt className="font-medium">{t("automations:mcpDesired")}</dt>
        <dd className="break-words">{names(evidence.desired)}</dd>
        <dt className="font-medium">{t("automations:mcpCurrentProcess")}</dt>
        <dd className="break-words">{names(evidence.bound)}</dd>
        <dt className="font-medium">{t("automations:mcpObserved")}</dt>
        <dd className="break-words">
          {evidence.observed.join(", ") || t("automations:mcpUnknown")}
        </dd>
      </dl>
      {evidence.pending ? (
        <p className="pb-2 text-muted-foreground">{t("automations:mcpNextStart")}</p>
      ) : null}
    </details>
  );
}
