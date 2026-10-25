"use client";

import { IconPlus, IconMinus, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { MCPSetupChange } from "@/lib/types/http-mcp";
import type { MCPSelectionChanges } from "./mcp-selection-changes";

const ACTIONS = {
  add: { icon: IconPlus, key: "settings:mcpPendingAdd" },
  remove: { icon: IconMinus, key: "settings:mcpPendingRemove" },
  update: { icon: IconRefresh, key: "settings:mcpPendingUpdate" },
};

function PendingAction({ change }: { change: MCPSetupChange }) {
  const { t } = useTranslation();
  const { icon: Icon, key } = ACTIONS[change.action];
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-muted-foreground">
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      {t(key)}
    </span>
  );
}

export function McpPendingChanges({ value }: { value: MCPSelectionChanges }) {
  const { t } = useTranslation();
  if (value.status !== "known" || value.changes.length === 0)
    return (
      <p
        data-testid="mcp-pending-comparison-message"
        className="pt-2 text-pretty text-xs text-muted-foreground"
      >
        {t(
          value.status === "known"
            ? "settings:mcpPendingNoDifference"
            : "settings:mcpPendingUnavailable",
        )}
      </p>
    );
  return (
    <section
      className="pt-3"
      data-testid="mcp-pending-changes"
      aria-labelledby="mcp-pending-changes-heading"
    >
      <h3
        id="mcp-pending-changes-heading"
        className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground"
      >
        {t("settings:mcpPendingChanges")}
        <span className="rounded bg-background/50 px-1.5 leading-4 tracking-normal tabular-nums">
          {value.changes.length}
        </span>
      </h3>
      <ul className="space-y-1">
        {value.changes.map((change) => (
          <li
            key={change.definition_id}
            data-testid={`mcp-pending-change-${change.definition_id}`}
            className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 gap-y-1 py-1.5 max-sm:grid-cols-1"
          >
            <span className="min-w-0 break-words text-[13px] font-medium">
              {(change.after ?? change.before)!.display_name}
            </span>
            <PendingAction change={change} />
          </li>
        ))}
      </ul>
    </section>
  );
}

export function McpPendingRowAnnotation({ change }: { change: MCPSetupChange }) {
  const { t } = useTranslation();
  return (
    <span className="mt-1.5 block" data-testid={`mcp-pending-row-${change.definition_id}`}>
      <PendingAction change={change} />
      {change.action === "remove" && (
        <span className="mt-0.5 block text-pretty text-xs text-muted-foreground">
          {t("settings:mcpPendingRemoveExplanation")}
        </span>
      )}
    </span>
  );
}
