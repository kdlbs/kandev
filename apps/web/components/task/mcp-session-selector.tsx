"use client";

import { useSearchParams } from "@/lib/routing/client-router";
import { readMCPContextHint } from "@/lib/mcp/management-links";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { IconChevronDown, IconX } from "@tabler/icons-react";
import { MCPSelectionSaveFeedback } from "@/components/mcp/mcp-selection-save-feedback";
import { MCPSelectionContent } from "@/components/mcp/mcp-selection-picker";
import { McpSelectionNotice } from "./chat/mcp-explorer/mcp-selection-notice";
import { mcpSelectionStateLabelKey } from "./chat/mcp-explorer/mcp-selection-presentation";
import { mcpSelectionChanges } from "./chat/mcp-explorer/mcp-selection-changes";
import { McpPendingChanges } from "./chat/mcp-explorer/mcp-selection-changes-view";
import {
  McpSessionRowAnnotation,
  McpDormantExclusions,
} from "./chat/mcp-explorer/mcp-session-exclusions";
import { MobilePickerSheet } from "./mobile/mobile-picker-sheet";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isHandledApiError } from "@/lib/api/client";
import type {
  MCPInheritedSelection,
  MCPServerDefinition,
  MCPSessionSelectionState,
} from "@/lib/types/http-mcp";

export type MCPSessionSelectorProps = {
  assignmentSessionId?: string;
  assignmentActive?: boolean;
  workspaceId?: string | null;
  definitions: MCPServerDefinition[];
  definitionsLoading?: boolean;
  selectedIds: string[];
  savedSelectedIds?: string[];
  excludedIds?: string[];
  onExcludedIdsChange?: (ids: string[]) => void;
  onSelectedIdsChange: (ids: string[]) => void;
  inherited?: MCPInheritedSelection[];
  state?: MCPSessionSelectionState | null;
  disabled?: boolean;
  retry?: () => void;
  reload?: () => Promise<unknown>;
  retrying?: boolean;
  error?: unknown;
  testId?: string;
};

export function MCPSessionSelectionBody({
  state,
  retry,
  retrying,
  definitionsLoading,
  reload,
  excludedIds = [],
  onExcludedIdsChange,
  ...props
}: MCPSessionSelectorProps & { query: string; onQueryChange: (query: string) => void }) {
  const { t } = useTranslation();
  const params = useSearchParams();
  const routeSession = readMCPContextHint(params, "sessionId");
  const changes = mcpSelectionChanges(
    state,
    props.savedSelectedIds ?? props.selectedIds,
    excludedIds,
  );
  const inheritedIds = new Set(props.inherited?.map((item) => item.definition.id));
  const dormant = onExcludedIdsChange
    ? props.definitions.filter(
        (item) => excludedIds.includes(item.id) && !inheritedIds.has(item.id),
      )
    : [];
  const dormantIds = new Set(dormant.map((item) => item.id));
  return (
    <div className="min-w-0 space-y-5">
      {state && (
        <div
          className={state.apply_state === "applied" ? "border-b border-border/60 pb-4" : undefined}
        >
          <McpSelectionNotice
            state={state}
            retry={retry}
            retrying={retrying}
            compact={state.apply_state === "applied"}
            changes={
              changes.status !== "hidden" ? <McpPendingChanges value={changes} /> : undefined
            }
          />
        </div>
      )}
      {reload && <MCPSelectionSaveFeedback error={props.error} reload={reload} />}
      {onExcludedIdsChange && (
        <>
          <div className="space-y-1.5 text-pretty text-xs text-muted-foreground">
            <p>{t("settings:mcpSessionExclusionsDescription")}</p>
            <p>{t("settings:mcpRequiredToolsSeparate")}</p>
            <p role="status">{t("settings:mcpExclusionCount", { total: excludedIds.length })}</p>
          </div>
          <McpDormantExclusions
            definitions={dormant}
            excludedIds={excludedIds}
            onChange={onExcludedIdsChange}
            disabled={props.disabled}
          />
        </>
      )}
      {definitionsLoading ? (
        <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
          {t("settings:mcpLoading")}
        </p>
      ) : (
        <MCPSelectionContent
          {...props}
          assignment={
            props.assignmentSessionId
              ? {
                  ownerKey: props.assignmentSessionId,
                  active: Boolean(
                    props.assignmentActive && routeSession.value === props.assignmentSessionId,
                  ),
                  ready: !definitionsLoading && !props.error,
                }
              : undefined
          }
          definitions={props.definitions.filter((item) => !dormantIds.has(item.id))}
          error={reload ? undefined : props.error}
          sessionPresentation
          renderServerAnnotation={(id) => (
            <McpSessionRowAnnotation
              change={changes.changes.find((item) => item.definition_id === id)}
              definition={props.inherited?.find((item) => item.definition.id === id)?.definition}
              excludedIds={excludedIds}
              onChange={onExcludedIdsChange}
              disabled={props.disabled}
            />
          )}
          sectionLabel={t("settings:mcpSessionAdditions")}
          testId={`${props.testId}-picker`}
        />
      )}
    </div>
  );
}

export function MCPSessionSelector({
  testId = "mcp-session-selector",
  ...props
}: MCPSessionSelectorProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const handledError = isHandledApiError(props.error);
  useEffect(() => {
    if (isMobile && handledError) setOpen(false);
  }, [isMobile, handledError]);
  const body = (
    <MCPSessionSelectionBody
      {...props}
      assignmentActive={open}
      testId={testId}
      query={query}
      onQueryChange={setQuery}
    />
  );
  const trigger = (
    <Button
      ref={triggerRef}
      type="button"
      variant="outline"
      className="w-full justify-between gap-3 cursor-pointer"
      disabled={handledError}
      aria-expanded={open}
      aria-haspopup={isMobile ? "dialog" : undefined}
      onClick={isMobile ? () => setOpen(true) : undefined}
    >
      <span className="min-w-0 truncate text-left">{t("settings:mcpSessionSelection")}</span>
      <span className="flex shrink-0 items-center gap-2">
        {props.state && (
          <Badge variant="outline">{t(mcpSelectionStateLabelKey(props.state))}</Badge>
        )}
        <Badge variant="secondary">
          {t("settings:mcpSelectedCount", { count: props.selectedIds.length })}
        </Badge>
        <IconChevronDown className={`size-4 transition-transform ${open ? "rotate-180" : ""}`} />
      </span>
    </Button>
  );
  if (isMobile)
    return (
      <div className="min-w-0" data-testid={testId}>
        {trigger}
        <MobilePickerSheet
          open={open}
          onOpenChange={setOpen}
          title={t("settings:mcpSessionSelection")}
          headerAction={
            <Button
              type="button"
              size="icon"
              variant="ghost"
              className="cursor-pointer"
              aria-label={t("common:close")}
              onClick={() => setOpen(false)}
            >
              <IconX className="size-4" />
            </Button>
          }
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            triggerRef.current?.focus();
          }}
        >
          {body}
        </MobilePickerSheet>
      </div>
    );
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="min-w-0" data-testid={testId}>
      <CollapsibleTrigger asChild>{trigger}</CollapsibleTrigger>
      <CollapsibleContent className="min-w-0 pt-3">{body}</CollapsibleContent>
    </Collapsible>
  );
}
