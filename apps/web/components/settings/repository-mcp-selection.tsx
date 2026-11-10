"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { IconChevronDown } from "@tabler/icons-react";
import { MCPSelectionContent } from "@/components/mcp/mcp-selection-picker";
import {
  SettingsSaveCancelledError,
  useSettingsSaveContributor,
} from "@/components/settings/settings-save-provider";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import { MCPSelectionSaveFeedback } from "@/components/mcp/mcp-selection-save-feedback";
import { useMCPSettingsSelectionDraft } from "@/hooks/domains/workspace/use-mcp-settings-selection-draft";
import { useSearchParams } from "@/lib/routing/client-router";
import { readMCPContextHint } from "@/lib/mcp/management-links";

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
  const draft = useMCPSettingsSelectionDraft("repository", repositoryId, workspaceId);
  const { editor, selectedIds, dirty, setSelectedIds } = draft;
  const params = useSearchParams();
  const hint = readMCPContextHint(params, "repositoryId");
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const section = useRef<HTMLDivElement>(null);
  const revealed = useRef(false);
  useEffect(() => {
    if (hint.value !== repositoryId || window.location.hash !== "#mcp" || revealed.current) return;
    revealed.current = true;
    setOpen(true);
    section.current?.scrollIntoView({ block: "start" });
    section.current?.focus({ preventScroll: true });
  }, [hint.value, repositoryId]);
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  const save = async () => {
    if (!(await draft.save())) throw new SettingsSaveCancelledError();
  };
  useSettingsSaveContributor({
    id: `repository-mcp-selection:${repositoryId}:${workspaceId}`,
    revision: JSON.stringify({ workspaceId, selectedIds }),
    isDirty: dirty,
    canSave:
      Boolean(editor.selection) &&
      !editor.loading &&
      !editor.saving &&
      !editor.loadError &&
      !editor.saveError &&
      !definitions.error,
    save,
    discard: draft.discard,
  });

  return (
    <Collapsible
      ref={section}
      tabIndex={-1}
      open={open}
      onOpenChange={setOpen}
      className="min-w-0 rounded-md border"
    >
      <RepositoryMCPSelectionTrigger open={open} count={selectedIds.length} />
      <CollapsibleContent
        className="min-w-0 border-t p-3"
        data-testid="repository-mcp-selection-content"
      >
        {definitions.loading || editor.loading ? (
          <p className="min-h-11 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
            {t("settings:mcpLoading")}
          </p>
        ) : (
          <div className="min-w-0 space-y-4">
            <p className="text-xs text-muted-foreground">
              {t("settings:mcpRepositorySelectionDescription")}
            </p>
            <MCPSelectionContent
              assignment={{
                ownerKey: repositoryId,
                active: open && hint.value === repositoryId,
                ready:
                  !definitions.loading &&
                  !editor.loading &&
                  !definitions.error &&
                  !editor.loadError,
              }}
              workspaceId={workspaceId}
              definitions={definitions.definitions}
              selectedIds={selectedIds}
              onSelectedIdsChange={setSelectedIds}
              disabled={Boolean(definitions.error || editor.loadError)}
              query={query}
              onQueryChange={setQuery}
              testId="repository-mcp-selection-picker"
            />
          </div>
        )}
        {Boolean(definitions.error || editor.loadError) && (
          <p className="mt-2 text-sm text-destructive" role="alert">
            {t("settings:mcpLoadFailed")}
          </p>
        )}
        <MCPSelectionSaveFeedback error={editor.saveError} reload={editor.reload} />
      </CollapsibleContent>
    </Collapsible>
  );
}

function RepositoryMCPSelectionTrigger({ open, count }: { open: boolean; count: number }) {
  const { t } = useTranslation();
  return (
    <CollapsibleTrigger asChild>
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 w-full justify-between gap-3 px-3 cursor-pointer"
        data-testid="repository-mcp-selection-trigger"
      >
        <span className="min-w-0 truncate text-sm font-medium">{t("settings:mcpServers")}</span>
        <span className="flex shrink-0 items-center gap-2">
          <Badge variant="secondary">{t("settings:mcpSelectedCount", { count: count })}</Badge>
          <IconChevronDown className={`size-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </span>
      </Button>
    </CollapsibleTrigger>
  );
}
