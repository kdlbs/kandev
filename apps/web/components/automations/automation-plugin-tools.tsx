"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  AutomationPluginToolGroup,
  AutomationPluginToolSelection,
} from "@/lib/types/automation";

type AutomationPluginToolsProps = {
  groups: AutomationPluginToolGroup[];
  loading: boolean;
  error: boolean;
  savedSelection: AutomationPluginToolSelection[];
  selection: AutomationPluginToolSelection[];
  onChange: (selection: AutomationPluginToolSelection[]) => void;
  onRetry: () => void;
  isDirty: boolean;
};

export function selectionMatchesGroup(
  selection: AutomationPluginToolSelection,
  group: AutomationPluginToolGroup,
): boolean {
  return (
    selection.plugin_id === group.plugin_id &&
    selection.installation_id === group.installation_id &&
    selection.policy_version === group.policy_version &&
    selection.tool_names.every((name) => group.tools.some((tool) => tool.local_name === name))
  );
}

export function selectedToolNames(
  selection: AutomationPluginToolSelection[],
  group: AutomationPluginToolGroup,
): string[] {
  return (
    selection
      .find((entry) => selectionMatchesGroup(entry, group))
      ?.tool_names.filter((name) => group.tools.some((tool) => tool.local_name === name)) ?? []
  );
}

function staleSelectionNames(
  savedSelection: AutomationPluginToolSelection[],
  groups: AutomationPluginToolGroup[],
): string[] {
  const names = new Set<string>();
  for (const saved of savedSelection) {
    const group = groups.find((candidate) => candidate.plugin_id === saved.plugin_id);
    if (!group || selectionMatchesGroup(saved, group)) continue;
    for (const name of saved.tool_names) names.add(name);
  }
  return [...names];
}

function retainCurrentSelection(
  selection: AutomationPluginToolSelection[],
  groups: AutomationPluginToolGroup[],
): AutomationPluginToolSelection[] {
  return selection.filter((entry) => groups.some((group) => selectionMatchesGroup(entry, group)));
}

function toggleTool(
  group: AutomationPluginToolGroup,
  toolName: string,
  selection: AutomationPluginToolSelection[],
  groups: AutomationPluginToolGroup[],
): AutomationPluginToolSelection[] {
  const currentSelection = retainCurrentSelection(selection, groups);
  const current = currentSelection.find((entry) => selectionMatchesGroup(entry, group));
  if (!current) {
    return [
      ...currentSelection.filter((entry) => entry.plugin_id !== group.plugin_id),
      {
        plugin_id: group.plugin_id,
        installation_id: group.installation_id,
        policy_version: group.policy_version,
        tool_names: [toolName],
      },
    ];
  }
  const names = current.tool_names.includes(toolName)
    ? current.tool_names.filter((name) => name !== toolName)
    : [...current.tool_names, toolName];
  if (names.length === 0) {
    return currentSelection.filter((entry) => entry.plugin_id !== group.plugin_id);
  }
  return currentSelection.map((entry) =>
    entry.plugin_id === group.plugin_id ? { ...current, tool_names: names } : entry,
  );
}

function clearGroup(
  group: Pick<AutomationPluginToolGroup, "plugin_id">,
  selection: AutomationPluginToolSelection[],
  groups: AutomationPluginToolGroup[],
): AutomationPluginToolSelection[] {
  return retainCurrentSelection(selection, groups).filter(
    (entry) => entry.plugin_id !== group.plugin_id,
  );
}

function PluginToolGroupSection({
  group,
  groups,
  selection,
  hasSavedSelection,
  onChange,
  minHeight,
}: {
  group: AutomationPluginToolGroup;
  groups: AutomationPluginToolGroup[];
  selection: AutomationPluginToolSelection[];
  hasSavedSelection: boolean;
  onChange: (selection: AutomationPluginToolSelection[]) => void;
  minHeight: string;
}) {
  const { t } = useTranslation();
  const checked = selectedToolNames(selection, group);
  const showClear = checked.length > 0 || hasSavedSelection;
  const clearHeight = minHeight === "min-h-11" ? "min-h-11" : "";
  return (
    <div className="space-y-1" data-testid="plugin-tool-group">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="break-words whitespace-normal text-sm font-medium">{group.display_name}</p>
          <p className="truncate text-xs text-muted-foreground">{group.version}</p>
        </div>
        {showClear ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className={`shrink-0 ${clearHeight}`}
            data-testid={`plugin-tools-clear-${group.plugin_id}`}
            onClick={() => onChange(clearGroup(group, selection, groups))}
          >
            {t("automations:pluginToolsClear")}
          </Button>
        ) : null}
      </div>
      {group.tools.map((tool) => {
        const isChecked = checked.includes(tool.local_name);
        return (
          <label
            key={tool.local_name}
            className={`flex min-w-0 cursor-pointer items-start gap-3 rounded-md px-1 py-1.5 hover:bg-muted/50 ${minHeight}`}
            data-testid={`plugin-tool-${group.plugin_id}-${tool.local_name}`}
          >
            <Checkbox
              className="mt-0.5"
              checked={isChecked}
              onCheckedChange={() =>
                onChange(toggleTool(group, tool.local_name, selection, groups))
              }
            />
            <span className="min-w-0 flex-1">
              <span className="flex flex-wrap items-center gap-2">
                <span className="break-all text-sm">{tool.local_name}</span>
                <Badge variant="secondary" className="shrink-0 text-[10px]">
                  {tool.read_only_hint
                    ? t("automations:pluginToolsRead")
                    : t("automations:pluginToolsWrite")}
                </Badge>
              </span>
              <span className="block text-xs text-muted-foreground">{tool.description}</span>
            </span>
          </label>
        );
      })}
    </div>
  );
}

function PluginToolStatus({
  loading,
  error,
  empty,
  onRetry,
}: {
  loading: boolean;
  error: boolean;
  empty: boolean;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <p
        className="text-xs text-muted-foreground"
        data-testid="automation-plugin-tools-loading"
        role="status"
      >
        {t("automations:pluginToolsLoading")}
      </p>
    );
  }
  if (error) {
    return (
      <div className="space-y-2" data-testid="automation-plugin-tools-error" role="alert">
        <p className="text-xs text-destructive">{t("automations:pluginToolsLoadError")}</p>
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          {t("automations:pluginToolsRetry")}
        </Button>
      </div>
    );
  }
  if (empty) {
    return (
      <p
        className="text-xs text-muted-foreground"
        data-testid="automation-plugin-tools-empty"
        role="status"
      >
        {t("automations:pluginToolsEmpty")}
      </p>
    );
  }
  return null;
}

function UnavailablePluginTools({
  selections,
  selection,
  groups,
  onChange,
  minHeight,
}: {
  selections: AutomationPluginToolSelection[];
  selection: AutomationPluginToolSelection[];
  groups: AutomationPluginToolGroup[];
  onChange: (selection: AutomationPluginToolSelection[]) => void;
  minHeight: string;
}) {
  const { t } = useTranslation();
  const clearHeight = minHeight === "min-h-11" ? "min-h-11" : "";
  return selections.map((entry) => (
    <div
      key={entry.plugin_id}
      className="flex min-w-0 items-center justify-between gap-2 rounded-md border border-dashed p-2"
      data-testid={`plugin-tools-unavailable-${entry.plugin_id}`}
    >
      <div className="min-w-0">
        <p className="break-words whitespace-normal text-sm font-medium">{entry.plugin_id}</p>
        <p className="break-words whitespace-normal text-xs text-muted-foreground">
          {t("automations:pluginToolsUnavailable")}: {entry.tool_names.join(", ")}
        </p>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={`shrink-0 ${clearHeight}`}
        onClick={() => onChange(clearGroup(entry, selection, groups))}
      >
        {t("automations:pluginToolsClear")}
      </Button>
    </div>
  ));
}

export function AutomationPluginTools({
  groups,
  loading,
  error,
  savedSelection,
  selection,
  onChange,
  onRetry,
  isDirty,
}: AutomationPluginToolsProps) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const minHeight = isMobile || !isFinePointer ? "min-h-11" : "min-h-7";
  const staleNames = useMemo(
    () => (loading || error ? [] : staleSelectionNames(savedSelection, groups)),
    [savedSelection, groups, loading, error],
  );
  const unavailable = useMemo(() => {
    if (loading || error) return [];
    const groupIDs = new Set(groups.map((group) => group.plugin_id));
    return selection.filter((entry) => !groupIDs.has(entry.plugin_id));
  }, [selection, groups, loading, error]);

  return (
    <div
      className="space-y-3 rounded-lg border bg-card p-4"
      data-testid="automation-plugin-tools"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
    >
      <div>
        <h3 className="text-sm font-medium">{t("automations:pluginToolsTitle")}</h3>
        <p className="text-xs text-muted-foreground">{t("automations:pluginToolsDescription")}</p>
      </div>
      {staleNames.length > 0 ? (
        <div
          className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-xs"
          data-testid="automation-plugin-tools-review"
        >
          <p className="font-medium">{t("automations:pluginToolsReviewRequired")}</p>
          <p className="text-muted-foreground">
            {t("automations:pluginToolsPreviouslySelected", {
              names: staleNames.join(", "),
            })}
          </p>
          <p className="mt-1 text-muted-foreground">{t("automations:pluginToolsReselect")}</p>
        </div>
      ) : null}
      <PluginToolStatus
        loading={loading}
        error={error}
        empty={!loading && !error && groups.length === 0}
        onRetry={onRetry}
      />
      {!error && !loading
        ? groups.map((group) => (
            <PluginToolGroupSection
              key={group.plugin_id}
              group={group}
              groups={groups}
              selection={selection}
              hasSavedSelection={savedSelection.some(
                (entry) => entry.plugin_id === group.plugin_id,
              )}
              onChange={onChange}
              minHeight={minHeight}
            />
          ))
        : null}
      <UnavailablePluginTools
        selections={unavailable}
        selection={selection}
        groups={groups}
        onChange={onChange}
        minHeight={minHeight}
      />
      {isDirty && savedSelection.length > 0 && selection.length === 0 && !loading && !error ? (
        <p className="text-xs text-muted-foreground" data-testid="automation-plugin-tools-cleared">
          {t("automations:pluginToolsClearedHint")}
        </p>
      ) : null}
    </div>
  );
}
