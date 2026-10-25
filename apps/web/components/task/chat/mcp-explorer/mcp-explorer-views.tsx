"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { MCPSessionSelectionBody } from "@/components/task/mcp-session-selector";
import type { TaskSessionMCPSelection } from "@/hooks/domains/session/use-task-session-mcp-selection";
import { McpSelectionNotice } from "./mcp-selection-notice";
import { mcpSelectionMarker } from "./mcp-selection-presentation";
import { cn } from "@/lib/utils";
import { BuiltinPackageSelector, type BuiltinPackageSelection } from "./builtin-package-selector";
import { BuiltinSessionEvidenceProvider } from "./builtin-session-evidence";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";

export type ExplorerViewsProps = {
  builtin?: BuiltinPackageSelection;
  builtinSession?: BuiltinMCPSessionView;
  selection?: TaskSessionMCPSelection | null;
  view: "connections" | "customize";
  onViewChange: (view: "connections" | "customize") => void;
  query: string;
  onQueryChange: (query: string) => void;
};

export function ExplorerViews({
  builtin,
  builtinSession,
  selection,
  view,
  onViewChange,
  query,
  onQueryChange,
  children,
  touch,
}: ExplorerViewsProps & { children: ReactNode; touch: boolean }) {
  const { t } = useTranslation();
  if (!selection) return children;
  return (
    <BuiltinSessionEvidenceProvider value={builtinEvidence(builtin, builtinSession, onViewChange)}>
      <Tabs
        value={view}
        onValueChange={(value) => onViewChange(value as "connections" | "customize")}
        className="h-full min-h-0 gap-4"
      >
        <TabsList
          variant="line"
          className={cn(
            "w-full shrink-0 justify-start border-b border-border/70 p-0",
            touch ? "!h-11" : "!h-7",
          )}
        >
          <TabsTrigger
            value="connections"
            className={cn(
              "flex-none rounded-none after:bottom-0",
              touch ? "min-h-11 px-3" : "h-7 px-3",
            )}
            data-testid="mcp-connections-tab"
          >
            {t("task:mcpConnectionsView")}
          </TabsTrigger>
          <TabsTrigger
            value="customize"
            className={cn(
              "flex-none rounded-none after:bottom-0",
              touch ? "min-h-11 px-3" : "h-7 px-3",
            )}
            data-testid="mcp-customize-tab"
          >
            {t("task:mcpCustomizeView")}
          </TabsTrigger>
        </TabsList>
        <TabsContent
          value="connections"
          forceMount
          className={view === "connections" ? "min-h-0 overflow-hidden" : "hidden"}
        >
          <div className="flex h-full min-h-0 flex-col gap-3">
            {selection.state && mcpSelectionMarker(selection.state) && (
              <div className="shrink-0 space-y-1">
                <h3 className="text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
                  {t("task:mcpSavedChanges")}
                </h3>
                <McpSelectionNotice
                  state={selection.state}
                  retry={selection.retry}
                  retrying={selection.retrying}
                />
              </div>
            )}
            <h3 className="shrink-0 text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
              {t("task:mcpCurrentAgent")}
            </h3>
            <div className="min-h-0 flex-1">{children}</div>
          </div>
        </TabsContent>
        <TabsContent
          value="customize"
          forceMount
          className={view === "customize" ? "min-h-0 overflow-hidden" : "hidden"}
        >
          <CustomizationBody
            assignmentActive={view === "customize"}
            builtin={builtin}
            selection={selection}
            query={query}
            onQueryChange={onQueryChange}
          />
        </TabsContent>
      </Tabs>
    </BuiltinSessionEvidenceProvider>
  );
}

function CustomizationBody({
  assignmentActive,
  builtin,
  selection,
  query,
  onQueryChange,
}: Pick<ExplorerViewsProps, "builtin" | "query" | "onQueryChange"> & {
  selection: TaskSessionMCPSelection;
  assignmentActive: boolean;
}) {
  return (
    <div
      data-testid="mcp-customization-scroll"
      className="h-full min-h-0 overflow-y-auto overscroll-contain pr-1"
    >
      <div className="mx-auto max-w-2xl pb-1">
        {builtin && <BuiltinPackageSelector {...builtin} />}
        {!builtin?.selected && (
          <MCPSessionSelectionBody
            {...selection}
            assignmentActive={assignmentActive}
            query={query}
            onQueryChange={onQueryChange}
            testId="mcp-session-customization"
          />
        )}
      </div>
    </div>
  );
}

function builtinEvidence(
  builtin: ExplorerViewsProps["builtin"],
  view: ExplorerViewsProps["builtinSession"],
  onViewChange: ExplorerViewsProps["onViewChange"],
) {
  if (!builtin) return null;
  return {
    view,
    manage: (id: NonNullable<ExplorerViewsProps["builtin"]>["selected"]) => {
      builtin.select(id);
      onViewChange("customize");
    },
    refresh: builtin.onSaved,
  };
}
