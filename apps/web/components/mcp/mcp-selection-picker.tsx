"use client";

import { useId, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { useMCPAssignmentFocus } from "@/hooks/domains/workspace/use-mcp-assignment-focus";
import { useTranslation } from "react-i18next";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { formatMCPOriginLabel, type MCPOriginContext } from "./mcp-origin-labels";
import { cn } from "@/lib/utils";
import { mcpSelectionErrorKey } from "./mcp-selection-feedback";
import { McpDefinitionDetails } from "./mcp-definition-details";
import { McpOriginSections, McpSectionHeading } from "./mcp-origin-sections";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { IconChevronDown, IconSearch } from "@tabler/icons-react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  MCPInheritedSelection,
  MCPServerDefinition,
  MCPSelectionOrigin,
} from "@/lib/types/http-mcp";

export type { MCPInheritedSelection } from "@/lib/types/http-mcp";

type MCPSelectionPickerProps = {
  assignment?: { ownerKey: string; active: boolean; ready: boolean };
  workspaceId?: string | null;
  definitions: MCPServerDefinition[];
  selectedIds: string[];
  onSelectedIdsChange: (ids: string[]) => void;
  inherited?: MCPInheritedSelection[];
  disabled?: boolean;
  error?: unknown;
  label?: string;
  description?: string;
  sectionLabel?: string;
  testId?: string;
  renderServerAnnotation?: (id: string) => ReactNode;
};

function InheritedServers({
  inherited,
  context,
  sessionPresentation,
  renderServerAnnotation,
}: {
  inherited: MCPInheritedSelection[];
  context: MCPOriginContext;
  renderServerAnnotation?: (id: string) => ReactNode;
  sessionPresentation?: boolean;
}) {
  const { t } = useTranslation();
  const labels: Record<MCPSelectionOrigin["scope"], string> = {
    profile: t("settings:mcpOriginProfile"),
    repository: t("settings:mcpOriginRepository"),
    task: t("settings:mcpOriginTask"),
    task_session: t("settings:mcpOriginSession"),
  };
  if (inherited.length === 0) return null;
  if (sessionPresentation)
    return (
      <McpOriginSections
        inherited={inherited}
        context={context}
        renderServerAnnotation={renderServerAnnotation}
      />
    );
  return (
    <section data-testid="mcp-inherited-group" className="space-y-2">
      <p className="text-xs font-medium text-muted-foreground">{t("settings:mcpInherited")}</p>
      <div className="space-y-2">
        {inherited.map((item) => (
          <div
            key={item.definition.id}
            data-testid={`mcp-inherited-server-${item.definition.id}`}
            data-mcp-definition-id={item.definition.id}
            tabIndex={-1}
            className="rounded-md border border-dashed p-3 data-[mcp-highlighted=true]:ring-2 data-[mcp-highlighted=true]:ring-primary/60"
          >
            <div className="flex items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-sm font-medium">
                {item.definition.display_name}
              </span>
              <Badge variant="secondary">{t("settings:mcpInheritedBadge")}</Badge>
            </div>
            <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
              {item.origins.map((origin) => (
                <span className="wrap-anywhere" key={`${origin.scope}:${origin.owner_id}`}>
                  {formatMCPOriginLabel(origin, labels[origin.scope], context)}
                </span>
              ))}
            </div>
            {renderServerAnnotation?.(item.definition.id)}
          </div>
        ))}
      </div>
    </section>
  );
}

function ServerRow({
  server,
  checked,
  disabled,
  onChange,
  sessionPresentation,
  annotation,
}: {
  server: MCPServerDefinition;
  annotation?: ReactNode;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
  sessionPresentation?: boolean;
}) {
  const { t } = useTranslation();
  const nameId = useId();
  const detailsId = useId();
  return (
    <label
      data-testid={`mcp-choice-${server.id}`}
      data-mcp-definition-id={server.id}
      tabIndex={-1}
      className={cn(
        "flex items-start gap-3 cursor-pointer hover:bg-muted/40 data-[mcp-highlighted=true]:ring-2 data-[mcp-highlighted=true]:ring-inset data-[mcp-highlighted=true]:ring-primary/60",
        sessionPresentation
          ? "min-h-7 px-3 py-2.5 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          : "min-h-11 rounded-md border p-3",
        sessionPresentation && checked && "bg-primary/5",
      )}
    >
      <Checkbox
        className="mt-0.5 shrink-0"
        aria-labelledby={nameId}
        aria-describedby={detailsId}
        checked={checked}
        disabled={disabled || (!server.enabled && !checked)}
        onCheckedChange={(value) => onChange(value === true)}
      />
      <span className="min-w-0 flex-1">
        <span
          id={nameId}
          className={cn(
            "block font-medium",
            sessionPresentation ? "wrap-anywhere text-[13px]" : "wrap-anywhere text-sm",
          )}
        >
          {server.display_name}
        </span>
        <span className="block wrap-anywhere text-xs text-muted-foreground">
          {server.runtime_name}
        </span>
        <span id={detailsId} className="block">
          <McpDefinitionDetails definition={server} />
        </span>
        {annotation}
      </span>
      {!server.enabled && <Badge variant="outline">{t("settings:mcpDisabled")}</Badge>}
    </label>
  );
}

function AdditionalServersHeading({
  sessionPresentation,
  sectionLabel,
}: {
  sessionPresentation?: boolean;
  sectionLabel?: string;
}) {
  const { t } = useTranslation();
  return sessionPresentation ? (
    <McpSectionHeading
      title={t("settings:mcpAdditionalServers")}
      description={t("settings:mcpAdditionalServersDescription")}
    />
  ) : (
    <p className="text-xs font-medium text-muted-foreground">
      {sectionLabel ?? t("settings:mcpAdditionalServers")}
    </p>
  );
}

function useMCPOriginContext(workspaceId?: string | null) {
  return useAppStore(
    useShallow((state) => ({
      workspaceId: workspaceId ?? state.workspaces.activeId,
      profiles: state.agentProfiles.items,
      repositories: state.repositories.itemsByWorkspaceId,
      tasks: state.kanban.tasks,
      workflows: state.workflows.items,
      sessions: state.taskSessions.items,
    })),
  );
}

function AssignmentFocus({
  container,
  workspaceId,
  definitions,
  assignment,
}: {
  container: RefObject<HTMLDivElement | null>;
  workspaceId: string;
  definitions: MCPServerDefinition[];
  assignment: NonNullable<MCPSelectionPickerProps["assignment"]>;
}) {
  useMCPAssignmentFocus({ container, workspaceId, definitions, ...assignment });
  return null;
}

export function MCPSelectionContent({
  assignment,
  workspaceId,
  definitions,
  selectedIds,
  onSelectedIdsChange,
  inherited = [],
  disabled,
  error,
  sectionLabel,
  sessionPresentation,
  query,
  onQueryChange,
  testId,
  renderServerAnnotation,
}: Omit<MCPSelectionPickerProps, "label" | "description"> & {
  sessionPresentation?: boolean;
  query: string;
  onQueryChange: (query: string) => void;
}) {
  const { t } = useTranslation();
  const context = useMCPOriginContext(workspaceId ?? definitions[0]?.workspace_id);
  const inheritedIds = useMemo(
    () => new Set((inherited ?? []).map((item) => item.definition.id)),
    [inherited],
  );
  const additions = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return definitions.filter((server) => {
      if (inheritedIds.has(server.id)) return false;
      if (!normalized) return true;
      return `${server.display_name} ${server.runtime_name}`.toLowerCase().includes(normalized);
    });
  }, [definitions, inheritedIds, query]);
  const setChecked = (id: string, checked: boolean) => {
    const next = checked ? [...selectedIds, id] : selectedIds.filter((item) => item !== id);
    onSelectedIdsChange([...new Set(next)]);
  };
  const errorKey = mcpSelectionErrorKey(error);
  const container = useRef<HTMLDivElement>(null);

  return (
    <div
      className={cn("min-h-0", sessionPresentation ? "space-y-6" : "space-y-4")}
      data-testid={testId}
      ref={container}
    >
      {assignment && workspaceId && (
        <AssignmentFocus
          container={container}
          workspaceId={workspaceId}
          definitions={definitions}
          assignment={assignment}
        />
      )}
      {errorKey && (
        <p className="text-sm text-destructive" role="alert">
          {t(errorKey)}
        </p>
      )}
      <InheritedServers
        inherited={inherited}
        context={context}
        renderServerAnnotation={renderServerAnnotation}
        sessionPresentation={sessionPresentation}
      />
      <section className={sessionPresentation ? "space-y-3" : "space-y-2"}>
        <AdditionalServersHeading
          sessionPresentation={sessionPresentation}
          sectionLabel={sectionLabel}
        />
        <div className="relative">
          <IconSearch className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder={t("settings:searchMcpServers")}
            aria-label={t("settings:searchMcpServers")}
            className="pl-9"
          />
        </div>
        <ServerChoices
          servers={additions}
          selectedIds={selectedIds}
          disabled={disabled}
          sessionPresentation={sessionPresentation}
          onChange={setChecked}
          renderServerAnnotation={renderServerAnnotation}
        />
      </section>
    </div>
  );
}

function ServerChoices({
  servers,
  selectedIds,
  disabled,
  sessionPresentation,
  onChange,
  renderServerAnnotation,
}: {
  servers: MCPServerDefinition[];
  renderServerAnnotation?: (id: string) => ReactNode;
  selectedIds: string[];
  disabled?: boolean;
  sessionPresentation?: boolean;
  onChange: (id: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  if (servers.length === 0)
    return (
      <p className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
        {t("settings:mcpSelectionEmpty")}
      </p>
    );
  return (
    <div
      className={
        sessionPresentation
          ? "divide-y divide-border/60 overflow-hidden rounded-lg border border-border/70"
          : "space-y-2"
      }
    >
      {servers.map((server) => (
        <ServerRow
          key={server.id}
          server={server}
          annotation={renderServerAnnotation?.(server.id)}
          sessionPresentation={sessionPresentation}
          checked={selectedIds.includes(server.id)}
          disabled={disabled}
          onChange={(checked) => onChange(server.id, checked)}
        />
      ))}
    </div>
  );
}

export function MCPSelectionPicker({
  assignment,
  workspaceId,
  definitions,
  selectedIds,
  onSelectedIdsChange,
  inherited = [],
  disabled,
  error,
  label,
  description,
  sectionLabel,
  testId,
}: MCPSelectionPickerProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const title = label ?? t("settings:mcpServers");
  const pickerBody = (
    <MCPSelectionContent
      assignment={assignment && { ...assignment, active: assignment.active && open }}
      workspaceId={workspaceId}
      query={query}
      onQueryChange={setQuery}
      definitions={definitions}
      selectedIds={selectedIds}
      onSelectedIdsChange={onSelectedIdsChange}
      inherited={inherited}
      disabled={disabled}
      error={error}
      sectionLabel={sectionLabel}
    />
  );
  return (
    <div className="min-w-0 space-y-2" data-testid={testId ?? "mcp-selection-picker"}>
      <Button
        type="button"
        variant="outline"
        className="flex w-full cursor-pointer items-center justify-between gap-3 px-3"
        onClick={() => setOpen(!open)}
        disabled={disabled}
        aria-expanded={open}
      >
        <span className="min-w-0 truncate text-left">{title}</span>
        <span className="flex shrink-0 items-center gap-2">
          <Badge variant="secondary">
            {t("settings:mcpSelectedCount", { count: selectedIds.length })}
          </Badge>
          <IconChevronDown className="size-4" />
        </span>
      </Button>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      {!isMobile && open && <div className="rounded-md border p-3">{pickerBody}</div>}
      {isMobile && (
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerContent className="h-[min(80dvh,42rem)] max-h-[80dvh] overflow-hidden pb-[env(safe-area-inset-bottom)]">
            <DrawerHeader>
              <DrawerTitle>{title}</DrawerTitle>
              <DrawerDescription>
                {description ?? t("settings:mcpSelectionDescription")}
              </DrawerDescription>
            </DrawerHeader>
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-4">
              {pickerBody}
            </div>
          </DrawerContent>
        </Drawer>
      )}
    </div>
  );
}
