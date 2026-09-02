"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { IconChevronDown, IconRefresh } from "@tabler/icons-react";
import { MCPSelectionPicker } from "@/components/mcp/mcp-selection-picker";
import type {
  MCPInheritedSelection,
  MCPServerDefinition,
  MCPSessionSelectionState,
} from "@/lib/types/http-mcp";

type MCPSessionSelectorProps = {
  definitions: MCPServerDefinition[];
  definitionsLoading?: boolean;
  selectedIds: string[];
  onSelectedIdsChange: (ids: string[]) => void;
  inherited?: MCPInheritedSelection[];
  state?: MCPSessionSelectionState | null;
  disabled?: boolean;
  retry?: () => void;
  retrying?: boolean;
  error?: unknown;
  testId?: string;
};

function stateLabel(state: MCPSessionSelectionState, t: (key: string) => string): string {
  switch (state.apply_state) {
    case "applied":
      return t("settings:mcpSessionStateApplied");
    case "pending_idle":
      return t("settings:mcpSessionStatePending");
    case "deferred_restart":
      return t("settings:mcpSessionStateDeferred");
    case "failed":
      return t("settings:mcpSessionStateFailed");
  }
}

function stateDescription(state: MCPSessionSelectionState, t: (key: string) => string): string {
  switch (state.apply_state) {
    case "failed":
      return t("settings:mcpSessionStateFailedDescription");
    case "pending_idle":
      return t("settings:mcpSessionStatePendingDescription");
    case "deferred_restart":
      return t("settings:mcpSessionStateDeferredDescription");
    case "applied":
      return t("settings:mcpSessionStateAppliedDescription");
  }
}

function SessionStateNotice({
  state,
  retry,
  retrying,
}: Pick<MCPSessionSelectorProps, "state" | "retry" | "retrying">) {
  const { t } = useTranslation();
  if (!state) return null;
  const failed = state.apply_state === "failed";
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3"
      role="status"
    >
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={failed ? "destructive" : "secondary"}>{stateLabel(state, t)}</Badge>
          <span className="text-xs text-muted-foreground">
            {t("settings:mcpSessionRevision", { revision: state.desired_revision })}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">{stateDescription(state, t)}</p>
      </div>
      {failed && retry && (
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer"
          onClick={retry}
          disabled={retrying}
        >
          <IconRefresh className="mr-2 size-4" />
          {t("settings:mcpSessionRetry")}
        </Button>
      )}
    </div>
  );
}

export function MCPSessionSelector({
  definitions,
  definitionsLoading = false,
  selectedIds,
  onSelectedIdsChange,
  inherited = [],
  state,
  disabled,
  retry,
  retrying,
  error,
  testId = "mcp-session-selector",
}: MCPSessionSelectorProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="min-w-0" data-testid={testId}>
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 w-full justify-between gap-3 cursor-pointer"
          disabled={disabled}
        >
          <span className="min-w-0 truncate text-left">{t("settings:mcpSessionSelection")}</span>
          <span className="flex shrink-0 items-center gap-2">
            <Badge variant="secondary">
              {t("settings:mcpSelectedCount", { count: selectedIds.length })}
            </Badge>
            <IconChevronDown
              className={`size-4 transition-transform ${open ? "rotate-180" : ""}`}
            />
          </span>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="min-w-0 space-y-3 pt-3">
        <SessionStateNotice state={state} retry={retry} retrying={retrying} />
        {Boolean(error) && (
          <p className="text-sm text-destructive" role="alert">
            {t("settings:mcpSaveFailed")}
          </p>
        )}
        {definitionsLoading ? (
          <p className="min-h-11 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
            {t("settings:mcpLoading")}
          </p>
        ) : (
          <MCPSelectionPicker
            definitions={definitions}
            selectedIds={selectedIds}
            onSelectedIdsChange={onSelectedIdsChange}
            inherited={inherited}
            disabled={disabled}
            label={t("settings:mcpServers")}
            description={t("settings:mcpSessionSelectionDescription")}
            testId={`${testId}-picker`}
          />
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
