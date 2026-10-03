"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { IconChevronDown } from "@tabler/icons-react";
import { MCPSelectionPicker } from "@/components/mcp/mcp-selection-picker";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import { useMCPSelectionEditor } from "@/hooks/domains/workspace/use-mcp-selection-editor";

function sameSelection(left: string[], right: string[]): boolean {
  if (left.length !== right.length) return false;
  const rightSet = new Set(right);
  return left.every((id) => rightSet.has(id));
}

export function RepositoryMCPSelectionSection({
  repositoryId,
  workspaceId,
  onDirtyChange,
}: {
  repositoryId: string;
  workspaceId: string;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t } = useTranslation();
  const definitions = useMCPWorkspaceDefinitions(workspaceId);
  const editor = useMCPSelectionEditor("repository", repositoryId, workspaceId);
  const [open, setOpen] = useState(false);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [baselineIds, setBaselineIds] = useState<string[]>([]);
  const selectionKey = editor.selection
    ? `${editor.selection.workspace_id}:${editor.selection.owner_id}:${editor.selection.definition_ids.join(",")}`
    : "empty";

  useEffect(() => {
    if (!editor.selection) return;
    setSelectedIds(editor.selection.definition_ids);
    setBaselineIds(editor.selection.definition_ids);
  }, [selectionKey]);

  const dirty = Boolean(editor.selection) && !sameSelection(selectedIds, baselineIds);
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  const save = async () => {
    await editor.save(selectedIds);
    setBaselineIds(selectedIds);
  };
  useSettingsSaveContributor({
    id: `repository-mcp-selection:${repositoryId}`,
    revision: JSON.stringify({ workspaceId, selectedIds }),
    isDirty: dirty,
    canSave:
      Boolean(editor.selection) &&
      !editor.loading &&
      !editor.saving &&
      !editor.loadError &&
      !definitions.error,
    save,
    discard: () => setSelectedIds(baselineIds),
  });

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="min-w-0 rounded-md border">
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="min-h-11 w-full justify-between gap-3 px-3 cursor-pointer"
          data-testid="repository-mcp-selection-trigger"
        >
          <span className="min-w-0 truncate text-sm font-medium">{t("settings:mcpServers")}</span>
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
      <CollapsibleContent
        className="min-w-0 border-t p-3"
        data-testid="repository-mcp-selection-content"
      >
        {definitions.loading || editor.loading ? (
          <p className="min-h-11 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
            {t("settings:mcpLoading")}
          </p>
        ) : (
          <MCPSelectionPicker
            definitions={definitions.definitions}
            selectedIds={selectedIds}
            onSelectedIdsChange={setSelectedIds}
            disabled={Boolean(definitions.error || editor.loadError)}
            label={t("settings:mcpServers")}
            description={t("settings:mcpSelectionDescription")}
            testId="repository-mcp-selection-picker"
          />
        )}
        {Boolean(definitions.error || editor.loadError) && (
          <p className="mt-2 text-sm text-destructive" role="alert">
            {t("settings:mcpLoadFailed")}
          </p>
        )}
        {Boolean(editor.saveError) && (
          <p className="mt-2 text-sm text-destructive" role="alert">
            {t("settings:mcpSaveFailed")}
          </p>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
