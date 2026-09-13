"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { MCPSelectionPicker } from "@/components/mcp/mcp-selection-picker";
import { useAppStore } from "@/components/state-provider";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import { useMCPSelectionEditor } from "@/hooks/domains/workspace/use-mcp-selection-editor";
import { SettingsCard } from "@/components/settings/settings-card";

function sameSelection(left: string[], right: string[]): boolean {
  if (left.length !== right.length) return false;
  const rightSet = new Set(right);
  return left.every((id) => rightSet.has(id));
}

type ProfileMCPSelectionContentProps = {
  profileId: string;
  hasWorkspaceProp: boolean;
  selectedWorkspaceId: string | null;
  selectedIds: string[];
  workspaces: { id: string; name: string }[];
  definitions: ReturnType<typeof useMCPWorkspaceDefinitions>;
  editor: ReturnType<typeof useMCPSelectionEditor>;
  workspaceRequired: boolean;
  onWorkspaceChange: (workspaceId: string) => void;
  onSelectedIdsChange: (ids: string[]) => void;
};

function ProfileMCPSelectionContent({
  profileId,
  hasWorkspaceProp,
  selectedWorkspaceId,
  selectedIds,
  workspaces,
  definitions,
  editor,
  workspaceRequired,
  onWorkspaceChange,
  onSelectedIdsChange,
}: ProfileMCPSelectionContentProps) {
  const { t } = useTranslation();
  const hasLoadError = Boolean(definitions.error || editor.loadError);
  let selectionContent: ReactNode;
  if (workspaceRequired) {
    selectionContent = (
      <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
        {t("agents:mcpWorkspaceRequired")}
      </p>
    );
  } else if (definitions.loading || editor.loading) {
    selectionContent = (
      <p className="min-h-11 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
        {t("settings:mcpLoading")}
      </p>
    );
  } else {
    selectionContent = (
      <MCPSelectionPicker
        definitions={definitions.definitions}
        selectedIds={selectedIds}
        onSelectedIdsChange={onSelectedIdsChange}
        disabled={hasLoadError}
        label={t("settings:mcpServers")}
        description={t("settings:mcpSelectionDescription")}
        testId={`profile-mcp-selection-${profileId}`}
      />
    );
  }
  return (
    <CardContent className="space-y-4">
      {!hasWorkspaceProp && (
        <div className="space-y-2">
          <Label htmlFor={`mcp-workspace-${profileId}`}>{t("agents:mcpWorkspace")}</Label>
          <Select value={selectedWorkspaceId ?? ""} onValueChange={onWorkspaceChange}>
            <SelectTrigger id={`mcp-workspace-${profileId}`} className="w-full">
              <SelectValue placeholder={t("agents:mcpSelectWorkspace")} />
            </SelectTrigger>
            <SelectContent>
              {workspaces.map((workspace) => (
                <SelectItem key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t("agents:mcpWorkspaceSelectionDescription")}
          </p>
        </div>
      )}
      {selectionContent}
      {hasLoadError && (
        <p className="text-sm text-destructive" role="alert">
          {t("settings:mcpLoadFailed")}
        </p>
      )}
      {Boolean(editor.saveError) && (
        <p className="text-sm text-destructive" role="alert">
          {t("settings:mcpSaveFailed")}
        </p>
      )}
    </CardContent>
  );
}

export function ProfileMCPSelectionCard({
  profileId,
  workspaceId,
}: {
  profileId: string;
  workspaceId?: string | null;
}) {
  const { t } = useTranslation();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState(workspaceId ?? null);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [baselineIds, setBaselineIds] = useState<string[]>([]);
  const definitions = useMCPWorkspaceDefinitions(selectedWorkspaceId);
  const editor = useMCPSelectionEditor("profile", profileId, selectedWorkspaceId);
  const selectionKey = editor.selection
    ? `${editor.selection.workspace_id}:${editor.selection.owner_id}:${editor.selection.definition_ids.join(",")}`
    : `${selectedWorkspaceId ?? "none"}:empty`;

  useEffect(() => setSelectedWorkspaceId(workspaceId ?? null), [workspaceId]);
  useEffect(() => {
    if (!editor.selection) {
      if (!selectedWorkspaceId) {
        setSelectedIds([]);
        setBaselineIds([]);
      }
      return;
    }
    setSelectedIds(editor.selection.definition_ids);
    setBaselineIds(editor.selection.definition_ids);
  }, [selectionKey, selectedWorkspaceId]);

  const dirty = Boolean(editor.selection) && !sameSelection(selectedIds, baselineIds);
  const workspaceRequired = !selectedWorkspaceId;
  const save = async () => {
    if (!selectedWorkspaceId) return;
    await editor.save(selectedIds);
    setBaselineIds(selectedIds);
  };
  useSettingsSaveContributor({
    id: `agent-profile-mcp-selection:${profileId}`,
    revision: JSON.stringify({ selectedWorkspaceId, selectedIds }),
    isDirty: dirty,
    canSave:
      !workspaceRequired &&
      Boolean(editor.selection) &&
      !editor.loading &&
      !editor.saving &&
      !editor.loadError &&
      !definitions.error,
    invalidReason: workspaceRequired ? t("agents:mcpWorkspaceRequired") : undefined,
    save,
    discard: () => setSelectedIds(baselineIds),
  });

  return (
    <SettingsCard isDirty={dirty}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t("agents:mcpConfiguration")}
          <Badge variant="secondary">{t("agents:mcpTypedSelection")}</Badge>
        </CardTitle>
      </CardHeader>
      <ProfileMCPSelectionContent
        profileId={profileId}
        hasWorkspaceProp={Boolean(workspaceId)}
        selectedWorkspaceId={selectedWorkspaceId}
        selectedIds={selectedIds}
        workspaces={workspaces}
        definitions={definitions}
        editor={editor}
        workspaceRequired={workspaceRequired}
        onWorkspaceChange={(value) => {
          setSelectedWorkspaceId(value);
          setSelectedIds([]);
          setBaselineIds([]);
        }}
        onSelectedIdsChange={setSelectedIds}
      />
    </SettingsCard>
  );
}
