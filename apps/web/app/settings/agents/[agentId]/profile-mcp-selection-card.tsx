"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  SettingsSaveCancelledError,
  useSettingsSaveCoordinator,
  useSettingsSaveContributor,
} from "@/components/settings/settings-save-provider";
import { MCPSelectionContent } from "@/components/mcp/mcp-selection-picker";
import { useAppStore } from "@/components/state-provider";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import { MCPSelectionSaveFeedback } from "@/components/mcp/mcp-selection-save-feedback";
import { useMCPSelectionEditor } from "@/hooks/domains/workspace/use-mcp-selection-editor";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { readMCPContextHint } from "@/lib/mcp/management-links";
import { useMCPSettingsSelectionDraft } from "@/hooks/domains/workspace/use-mcp-settings-selection-draft";
import { SettingsCard } from "@/components/settings/settings-card";

type ProfileMCPSelectionContentProps = {
  profileId: string;
  hasWorkspaceProp: boolean;
  selectedWorkspaceId: string | null;
  selectedIds: string[];
  workspaces: { id: string; name: string }[];
  definitions: ReturnType<typeof useMCPWorkspaceDefinitions>;
  editor: ReturnType<typeof useMCPSelectionEditor>;
  workspaceRequired: boolean;
  switchingDisabled: boolean;
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
  switchingDisabled,
  onWorkspaceChange,
  onSelectedIdsChange,
}: ProfileMCPSelectionContentProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
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
      <MCPSelectionContent
        query={query}
        onQueryChange={setQuery}
        assignment={{
          ownerKey: profileId,
          active: !workspaceRequired,
          ready:
            Boolean(editor.selection) && !definitions.loading && !editor.loading && !hasLoadError,
        }}
        workspaceId={selectedWorkspaceId ?? undefined}
        definitions={definitions.definitions}
        selectedIds={selectedIds}
        onSelectedIdsChange={onSelectedIdsChange}
        disabled={hasLoadError}
        sectionLabel={t("settings:mcpProfileSelectionDescription")}
        testId={`profile-mcp-selection-${profileId}`}
      />
    );
  }
  return (
    <CardContent className="space-y-4">
      {!hasWorkspaceProp && (
        <div className="space-y-2">
          <Label htmlFor={`mcp-workspace-${profileId}`}>{t("agents:mcpWorkspace")}</Label>
          <Select
            disabled={switchingDisabled}
            value={selectedWorkspaceId ?? ""}
            onValueChange={onWorkspaceChange}
          >
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
      <MCPSelectionSaveFeedback error={editor.saveError} reload={editor.reload} />
    </CardContent>
  );
}

function useProfileMCPWorkspace(workspaceId?: string | null) {
  const workspaces = useAppStore((state) => state.workspaces.items);
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const hint = readMCPContextHint(params, "workspaceId");
  const hintAllowed =
    !hint.present ||
    Boolean(
      hint.value &&
      workspaces.some((w) => w.id === hint.value) &&
      (!workspaceId || hint.value === workspaceId),
    );
  const selectedWorkspaceId = hintAllowed ? workspaceId || hint.value : null;
  const changeWorkspace = (value: string) => {
    if (value === selectedWorkspaceId || !workspaces.some((w) => w.id === value)) return;
    const next = new URLSearchParams(params);
    next.set("workspaceId", value);
    router.replace(`${pathname}?${next}#mcp`, { scroll: false });
  };
  return { workspaces, hintAllowed, selectedWorkspaceId, changeWorkspace };
}

function useProfileMCPDraft(
  profileId: string,
  selectedWorkspaceId: string | null,
  definitions: ReturnType<typeof useMCPWorkspaceDefinitions>,
) {
  const draft = useMCPSettingsSelectionDraft("profile", profileId, selectedWorkspaceId);
  const { editor, selectedIds, dirty } = draft;
  useSettingsSaveContributor({
    id: `agent-profile-mcp-selection:${profileId}:${selectedWorkspaceId ?? ""}`,
    revision: JSON.stringify({ selectedWorkspaceId, selectedIds }),
    isDirty: dirty,
    canSave:
      Boolean(editor.selection) &&
      !editor.loading &&
      !editor.saving &&
      !editor.loadError &&
      !editor.saveError &&
      !definitions.error,
    save: async () => {
      if (!(await draft.save())) throw new SettingsSaveCancelledError();
    },
    discard: draft.discard,
  });
  return draft;
}

export function ProfileMCPSelectionCard({
  profileId,
  workspaceId,
  legacyEditor,
}: {
  profileId: string;
  workspaceId?: string | null;
  legacyEditor?: ReactNode;
}) {
  const { t } = useTranslation();
  const { workspaces, hintAllowed, selectedWorkspaceId, changeWorkspace } =
    useProfileMCPWorkspace(workspaceId);
  const coordinator = useSettingsSaveCoordinator();
  const definitions = useMCPWorkspaceDefinitions(selectedWorkspaceId);
  const draft = useProfileMCPDraft(profileId, selectedWorkspaceId, definitions);
  const { editor, selectedIds, dirty, setSelectedIds } = draft;
  const [legacyOpen, setLegacyOpen] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const revealed = useRef<string | null>(null);
  useEffect(() => {
    const key = JSON.stringify([profileId, selectedWorkspaceId]);
    if (!editor.selection || window.location.hash !== "#mcp" || revealed.current === key) return;
    revealed.current = key;
    heading.current?.scrollIntoView({ block: "start" });
    heading.current?.focus({ preventScroll: true });
  }, [profileId, selectedWorkspaceId, editor.selection]);
  const showLegacy =
    !workspaceId &&
    (legacyOpen ||
      (editor.selection !== null && editor.selection.legacy_import_status !== "complete"));
  return (
    <>
      <SettingsCard isDirty={dirty} id="mcp">
        <CardHeader>
          <CardTitle ref={heading} tabIndex={-1}>
            {t("settings:mcpServers")}
          </CardTitle>
        </CardHeader>
        {!hintAllowed && (
          <p role="alert" className="px-6 text-sm text-muted-foreground">
            {t("settings:mcpOwnerUnavailable")}
          </p>
        )}
        <ProfileMCPSelectionContent
          key={selectedWorkspaceId}
          profileId={profileId}
          hasWorkspaceProp={Boolean(workspaceId)}
          selectedWorkspaceId={selectedWorkspaceId}
          selectedIds={selectedIds}
          workspaces={workspaces}
          definitions={definitions}
          editor={editor}
          workspaceRequired={!selectedWorkspaceId}
          switchingDisabled={
            editor.saving || coordinator.status === "saving" || coordinator.exclusiveBusy
          }
          onSelectedIdsChange={setSelectedIds}
          onWorkspaceChange={changeWorkspace}
        />
        {showLegacy && (
          <div className="space-y-2 border-t px-6 py-4">
            <p className="text-sm text-muted-foreground">
              {t("settings:mcpLegacyRecoveryDescription")}
            </p>
            {legacyEditor && !legacyOpen && (
              <Button
                variant="outline"
                className={controlSizingClassName(
                  "standard",
                  "h-auto max-w-full cursor-pointer whitespace-normal text-left",
                )}
                onClick={() => setLegacyOpen(true)}
              >
                {t("settings:mcpLegacyRecovery")}
              </Button>
            )}
          </div>
        )}
      </SettingsCard>
      {legacyOpen && legacyEditor && (
        <section aria-label={t("settings:mcpLegacyRecovery")} className="space-y-2">
          <h3 className="text-sm font-medium">{t("settings:mcpLegacyRecovery")}</h3>
          {legacyEditor}
        </section>
      )}
    </>
  );
}
