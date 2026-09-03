"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { MCPSelectionPicker } from "@/components/mcp/mcp-selection-picker";
import { useAppStore } from "@/components/state-provider";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import { useMCPSelectionEditor } from "@/hooks/domains/workspace/use-mcp-selection-editor";
// `validateDraftServers` runs outside React (from an onChange handler and from
// the parent's draft state), so it uses the module-level `t`, which resolves at
// call time. Components in this file use the `useTranslation()` hook.
import { t as translate } from "@/lib/i18n";
import { useProfileMcpConfig } from "./use-profile-mcp-config";
import type { AgentProfileMcpConfig } from "@/lib/types/http";

type ProfileMcpConfigCardProps = {
  profileId: string;
  workspaceId?: string | null;
  supportsMcp: boolean;
  /**
   * Whether the profile is in CLI passthrough mode. When true (and
   * mcpInjection is set), the card explains how kandev injects MCP servers
   * into the agent's CLI.
   */
  cliPassthrough?: boolean;
  /**
   * Human-readable phrase describing the passthrough MCP injection mechanism
   * (from PassthroughConfig.mcp_injection). Only rendered when cliPassthrough.
   */
  mcpInjection?: string;
  initialConfig?: AgentProfileMcpConfig | null;
  draftState?: {
    enabled: boolean;
    servers: string;
    dirty: boolean;
    error: string | null;
  };
  onDraftStateChange?: (next: {
    enabled?: boolean;
    servers?: string;
    dirty?: boolean;
    error?: string | null;
  }) => void;
  onToastError: (error: unknown) => void;
};

// i18n-exempt: MCP server config the user copies verbatim, including the example header value.
const POPULAR_SERVERS: Record<string, Record<string, unknown>> = {
  playwright: {
    type: "stdio",
    command: "npx",
    args: ["-y", "@playwright/mcp"],
  },
  "chrome-devtools": {
    type: "stdio",
    command: "npx",
    args: ["-y", "chrome-devtools-mcp"],
  },
  context7: {
    type: "stdio",
    command: "npx",
    args: ["-y", "@upstash/context7-mcp"],
    env: {
      CONTEXT7_API_KEY: "your_api_key_here",
    },
  },
  github: {
    type: "http",
    url: "https://api.githubcopilot.com/mcp/",
    headers: {
      Authorization: "Bearer your_token_here",
    },
  },
};

// Tool names and MCP-server product names are identifiers, not copy: they are
// interpolated as values so the pseudo-locale cannot turn them into something
// the user cannot type or look up.
// i18n-exempt: server name and tool ids are config values the user copies verbatim into an agent.
const KANDEV_MCP_NAME = "Kandev MCP";

// i18n-exempt: server name and tool ids are config values the user copies verbatim into an agent.
const KANDEV_TOOL_NAMES =
  "list_workspaces, list_boards, list_workflow_steps, list_tasks, create_task, update_task";

// i18n-exempt: MCP server product names.
const POPULAR_SERVER_NAMES: Record<string, string> = {
  playwright: "Playwright MCP",
  "chrome-devtools": "Chrome DevTools MCP",
  context7: "Context7 MCP",
  github: "GitHub MCP",
};

// The JSON key the editor validates against. Interpolated into the error
// messages below rather than written into the catalog.
const MCP_SERVERS_KEY = "mcpServers";

type PopularServerButtonProps = {
  label: string;
  displayName: string;
  onApply: (label: string) => void;
};

function PopularServerButton({ label, displayName, onApply }: PopularServerButtonProps) {
  return (
    <button
      type="button"
      className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
      onClick={() => onApply(label)}
    >
      + {displayName}
    </button>
  );
}

function applyPopularServerToJson(
  currentServers: string,
  label: string,
  isDraft: boolean,
  onDraftStateChange?: (next: { servers?: string; dirty?: boolean; error?: string | null }) => void,
  handleMcpServersChange?: (value: string) => void,
) {
  const base = currentServers.trim() || '{\n  "mcpServers": {}\n}';
  let parsed: Record<string, unknown> = {};
  try {
    parsed = JSON.parse(base) as Record<string, unknown>;
  } catch {
    return;
  }
  const root =
    parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : { mcpServers: {} };
  const servers = (
    root.mcpServers && typeof root.mcpServers === "object" && !Array.isArray(root.mcpServers)
      ? root.mcpServers
      : {}
  ) as Record<string, unknown>;

  if (servers[label]) return;
  servers[label] = POPULAR_SERVERS[label] ?? { type: "stdio", command: "npx", args: ["-y"] };
  root.mcpServers = servers;
  const nextValue = JSON.stringify(root, null, 2);

  if (isDraft) {
    onDraftStateChange?.({ servers: nextValue, dirty: true, error: null });
    return;
  }
  handleMcpServersChange?.(nextValue);
}

function validateDraftServers(value: string): string | null {
  if (!value.trim()) return null;
  try {
    const parsed = JSON.parse(value);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return translate("agents:mcpConfigMustBeJsonObject");
    }
    if (MCP_SERVERS_KEY in parsed) {
      const nested = (parsed as { mcpServers?: unknown }).mcpServers;
      if (!nested || typeof nested !== "object" || Array.isArray(nested)) {
        return translate("agents:mcpKeyMustBeJsonObject", { key: MCP_SERVERS_KEY });
      }
    }
  } catch {
    return translate("agents:invalidJson");
  }
  return null;
}

function PassthroughMcpInjectionHint({
  cliPassthrough,
  mcpInjection,
}: {
  cliPassthrough?: boolean;
  mcpInjection?: string;
}) {
  const { t } = useTranslation();
  if (!cliPassthrough || !mcpInjection) return null;
  return (
    <p className="text-xs text-muted-foreground">
      {t("agents:mcpPassthroughInjectionHint", { mechanism: mcpInjection })}
    </p>
  );
}

type McpServersEditorProps = {
  profileId: string;
  currentServers: string;
  currentError: string | null;
  isDirty: boolean;
  isDraft: boolean;
  isEditableProfile: boolean;
  cliPassthrough?: boolean;
  mcpInjection?: string;
  onDraftStateChange?: (next: { servers?: string; dirty?: boolean; error?: string | null }) => void;
  handleMcpServersChange: (value: string) => void;
};

function McpServersEditor({
  profileId,
  currentServers,
  currentError,
  isDirty,
  isDraft,
  isEditableProfile,
  cliPassthrough,
  mcpInjection,
  onDraftStateChange,
  handleMcpServersChange,
}: McpServersEditorProps) {
  const { t } = useTranslation();
  const handleApplyServer = (label: string) => {
    applyPopularServerToJson(
      currentServers,
      label,
      isDraft,
      onDraftStateChange,
      handleMcpServersChange,
    );
  };

  const handleDraftChange = (value: string) => {
    if (!onDraftStateChange) return;
    const error = validateDraftServers(value);
    onDraftStateChange({ servers: value, dirty: true, error });
  };

  return (
    <div className="space-y-2">
      <Label htmlFor={`mcp-servers-${profileId}`}>{t("agents:mcpServersJson")}</Label>
      <Textarea
        id={`mcp-servers-${profileId}`}
        className="min-h-[200px] font-mono text-xs"
        value={currentServers}
        onChange={(event) => {
          if (isDraft) {
            handleDraftChange(event.target.value);
            return;
          }
          handleMcpServersChange(event.target.value);
        }}
        disabled={!isEditableProfile && !isDraft}
        data-settings-dirty={isDirty}
        data-testid={`mcp-servers-${profileId}`}
      />
      <p className="text-xs text-muted-foreground">{t("agents:mcpServersHelp")}</p>
      <PassthroughMcpInjectionHint cliPassthrough={cliPassthrough} mcpInjection={mcpInjection} />
      <p className="text-xs font-medium text-muted-foreground">{t("agents:builtIn")}</p>
      <div className="flex flex-wrap gap-2 mb-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-xs rounded-full border border-primary/50 bg-primary/10 px-2 py-1 text-primary">
              {t("agents:kandevMcpChip", { name: KANDEV_MCP_NAME })}
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom" className="max-w-[320px] text-xs">
            <p className="font-medium mb-1">{t("agents:automaticallyAvailable")}</p>
            <p>{t("agents:kandevMcpTools", { tools: KANDEV_TOOL_NAMES })}</p>
          </TooltipContent>
        </Tooltip>
      </div>
      <p className="text-xs font-medium text-muted-foreground">{t("agents:popularServers")}</p>
      <div className="flex flex-wrap gap-2">
        {Object.keys(POPULAR_SERVERS).map((label) => (
          <PopularServerButton
            key={label}
            label={label}
            displayName={POPULAR_SERVER_NAMES[label] ?? label}
            onApply={handleApplyServer}
          />
        ))}
      </div>
      {currentError && <p className="text-sm text-destructive">{currentError}</p>}
    </div>
  );
}

type McpConfigState = {
  isDraft: boolean;
  isEditableProfile: boolean;
  currentEnabled: boolean;
  currentServers: string;
  currentError: string | null;
  currentDirty: boolean;
  enabledDirty: boolean;
  serversDirty: boolean;
};

type ResolveMcpConfigInput = {
  draftState: ProfileMcpConfigCardProps["draftState"];
  profileId: string;
  mcpEnabled: boolean;
  mcpServers: string;
  mcpBaselineEnabled: boolean;
  mcpBaselineServers: string;
  mcpError: string | null;
};

function resolveMcpConfigState(input: ResolveMcpConfigInput): McpConfigState {
  const isDraft = Boolean(input.draftState);
  const isEditableProfile =
    !isDraft && Boolean(input.profileId) && !input.profileId.startsWith("draft-");
  const baselineEnabled = isDraft ? false : input.mcpBaselineEnabled;
  const baselineServers = isDraft ? '{\n  "mcpServers": {}\n}' : input.mcpBaselineServers;
  const currentEnabled = isDraft ? (input.draftState?.enabled ?? false) : input.mcpEnabled;
  const currentServers = isDraft ? (input.draftState?.servers ?? "") : input.mcpServers;
  const enabledDirty = currentEnabled !== baselineEnabled;
  const serversDirty = currentServers !== baselineServers;
  return {
    isDraft,
    isEditableProfile,
    currentEnabled,
    currentServers,
    currentError: isDraft ? (input.draftState?.error ?? null) : input.mcpError,
    currentDirty: enabledDirty || serversDirty,
    enabledDirty,
    serversDirty,
  };
}

type McpEnableToggleProps = {
  currentEnabled: boolean;
  isDirty: boolean;
  isDraft: boolean;
  isEditableProfile: boolean;
  onDraftStateChange?: (next: { enabled?: boolean; dirty?: boolean }) => void;
  setMcpEnabled: (enabled: boolean) => void;
};

function McpEnableToggle({
  currentEnabled,
  isDirty,
  isDraft,
  isEditableProfile,
  onDraftStateChange,
  setMcpEnabled,
}: McpEnableToggleProps) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center justify-between rounded-md border p-3"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
      data-testid="mcp-enabled-row"
    >
      <div className="space-y-1">
        <Label>{t("agents:enableMcp")}</Label>
        <p className="text-xs text-muted-foreground">{t("agents:enableMcpDescription")}</p>
      </div>
      <Switch
        checked={currentEnabled}
        data-settings-dirty={isDirty}
        data-testid="mcp-enabled"
        onCheckedChange={(checked) => {
          if (isDraft) {
            onDraftStateChange?.({ enabled: checked, dirty: true });
            return;
          }
          setMcpEnabled(checked);
        }}
        disabled={!isEditableProfile && !isDraft}
      />
    </div>
  );
}

function McpProfileHint({
  isDraft,
  isEditableProfile,
}: {
  isDraft: boolean;
  isEditableProfile: boolean;
}) {
  const { t } = useTranslation();
  if (isEditableProfile) return null;
  return (
    <p className="text-xs text-muted-foreground">
      {isDraft ? t("agents:mcpDraftHint") : t("agents:mcpSaveProfileHint")}
    </p>
  );
}

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

function ProfileMCPSelectionCard({
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

export function ProfileMcpConfigCard({
  profileId,
  workspaceId,
  supportsMcp,
  cliPassthrough,
  mcpInjection,
  initialConfig,
  draftState,
  onDraftStateChange,
  onToastError,
}: ProfileMcpConfigCardProps) {
  const { t } = useTranslation();
  const {
    mcpEnabled,
    mcpServers,
    mcpBaselineEnabled,
    mcpBaselineServers,
    mcpError,
    setMcpEnabled,
    handleMcpServersChange,
    handleSaveMcp,
    resetMcpDraft,
  } = useProfileMcpConfig({ profileId, supportsMcp, initialConfig, onToastError });

  const state = resolveMcpConfigState({
    draftState,
    profileId,
    mcpEnabled,
    mcpServers,
    mcpBaselineEnabled,
    mcpBaselineServers,
    mcpError,
  });
  useSettingsSaveContributor({
    id: `agent-profile-mcp:${profileId}`,
    revision: JSON.stringify({
      enabled: state.currentEnabled,
      servers: state.currentServers,
    }),
    isDirty: supportsMcp && state.isEditableProfile && state.currentDirty,
    canSave: !state.currentError,
    invalidReason: state.currentError ?? undefined,
    save: handleSaveMcp,
    discard: resetMcpDraft,
  });

  if (!supportsMcp) return null;

  if (!state.isDraft && workspaceId) {
    return <ProfileMCPSelectionCard profileId={profileId} workspaceId={workspaceId} />;
  }

  return (
    <SettingsCard isDirty={state.currentDirty}>
      <CardHeader>
        <CardTitle>{t("agents:mcpConfiguration")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <McpProfileHint isDraft={state.isDraft} isEditableProfile={state.isEditableProfile} />
        <McpEnableToggle
          currentEnabled={state.currentEnabled}
          isDirty={state.enabledDirty}
          isDraft={state.isDraft}
          isEditableProfile={state.isEditableProfile}
          onDraftStateChange={onDraftStateChange}
          setMcpEnabled={setMcpEnabled}
        />
        <McpServersEditor
          profileId={profileId}
          currentServers={state.currentServers}
          currentError={state.currentError}
          isDirty={state.serversDirty}
          isDraft={state.isDraft}
          isEditableProfile={state.isEditableProfile}
          cliPassthrough={cliPassthrough}
          mcpInjection={mcpInjection}
          onDraftStateChange={onDraftStateChange}
          handleMcpServersChange={handleMcpServersChange}
        />
      </CardContent>
    </SettingsCard>
  );
}
