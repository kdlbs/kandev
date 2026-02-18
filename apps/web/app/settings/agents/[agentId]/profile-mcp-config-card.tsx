'use client';

import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Label } from '@kandev/ui/label';
import { Switch } from '@kandev/ui/switch';
import { Textarea } from '@kandev/ui/textarea';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import { useProfileMcpConfig } from './use-profile-mcp-config';
import type { AgentProfileMcpConfig } from '@/lib/types/http';

type ProfileMcpConfigCardProps = {
  profileId: string;
  supportsMcp: boolean;
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

const POPULAR_SERVERS: Record<string, Record<string, unknown>> = {
  playwright: {
    type: 'stdio',
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-playwright'],
  },
  'chrome-devtools': {
    type: 'stdio',
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-chrome-devtools'],
  },
  context7: {
    type: 'stdio',
    command: 'npx',
    args: ['-y', '@context7/mcp'],
    env: {
      CONTEXT7_API_KEY: 'your_api_key_here',
    },
  },
  github: {
    type: 'stdio',
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-github'],
    env: {
      GITHUB_TOKEN: 'your_token_here',
    },
  },
};

const KANDEV_TOOLS_DESCRIPTION = 'Tools: list_workspaces, list_boards, list_workflow_steps, list_tasks, create_task, update_task';

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
    parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed
      : { mcpServers: {} };
  const servers = (root.mcpServers &&
    typeof root.mcpServers === 'object' &&
    !Array.isArray(root.mcpServers)
    ? root.mcpServers
    : {}) as Record<string, unknown>;

  if (servers[label]) return;
  servers[label] = POPULAR_SERVERS[label] ?? { type: 'stdio', command: 'npx', args: ['-y'] };
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
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return 'MCP servers config must be a JSON object';
    }
    if ('mcpServers' in parsed) {
      const nested = (parsed as { mcpServers?: unknown }).mcpServers;
      if (!nested || typeof nested !== 'object' || Array.isArray(nested)) {
        return 'mcpServers must be a JSON object';
      }
    }
  } catch {
    return 'Invalid JSON';
  }
  return null;
}

type McpServersEditorProps = {
  profileId: string;
  currentServers: string;
  currentError: string | null;
  isDraft: boolean;
  isEditableProfile: boolean;
  onDraftStateChange?: (next: { servers?: string; dirty?: boolean; error?: string | null }) => void;
  handleMcpServersChange: (value: string) => void;
};

function McpServersEditor({
  profileId,
  currentServers,
  currentError,
  isDraft,
  isEditableProfile,
  onDraftStateChange,
  handleMcpServersChange,
}: McpServersEditorProps) {
  const handleApplyServer = (label: string) => {
    applyPopularServerToJson(currentServers, label, isDraft, onDraftStateChange, handleMcpServersChange);
  };

  const handleDraftChange = (value: string) => {
    if (!onDraftStateChange) return;
    const error = validateDraftServers(value);
    onDraftStateChange({ servers: value, dirty: true, error });
  };

  return (
    <div className="space-y-2">
      <Label htmlFor={`mcp-servers-${profileId}`}>MCP servers (JSON)</Label>
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
      />
      <p className="text-xs text-muted-foreground">
        MCP definitions are stored in the database and resolved per executor at runtime. This does not override your local agent config.</p>
      <p className="text-xs font-medium text-muted-foreground">Built-in</p>
      <div className="flex flex-wrap gap-2 mb-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="text-xs rounded-full border border-primary/50 bg-primary/10 px-2 py-1 text-primary"
            >
              ✓ Kandev MCP
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom" className="max-w-[320px] text-xs">
            <p className="font-medium mb-1">Automatically available</p>
            <p>{KANDEV_TOOLS_DESCRIPTION}</p>
          </TooltipContent>
        </Tooltip>
      </div>
      <p className="text-xs font-medium text-muted-foreground">Popular servers</p>
      <div className="flex flex-wrap gap-2">
        <PopularServerButton label="playwright" displayName="Playwright MCP" onApply={handleApplyServer} />
        <PopularServerButton label="chrome-devtools" displayName="Chrome DevTools MCP" onApply={handleApplyServer} />
        <PopularServerButton label="context7" displayName="Context7 MCP" onApply={handleApplyServer} />
        <PopularServerButton label="github" displayName="GitHub MCP" onApply={handleApplyServer} />
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
};

type ResolveMcpConfigInput = {
  draftState: ProfileMcpConfigCardProps['draftState'];
  profileId: string;
  mcpEnabled: boolean;
  mcpServers: string;
  mcpError: string | null;
  mcpDirty: boolean;
};

function resolveMcpConfigState(input: ResolveMcpConfigInput): McpConfigState {
  const isDraft = Boolean(input.draftState);
  const isEditableProfile = !isDraft && Boolean(input.profileId) && !input.profileId.startsWith('draft-');
  return {
    isDraft,
    isEditableProfile,
    currentEnabled: isDraft ? input.draftState?.enabled ?? false : input.mcpEnabled,
    currentServers: isDraft ? input.draftState?.servers ?? '' : input.mcpServers,
    currentError: isDraft ? input.draftState?.error ?? null : input.mcpError,
    currentDirty: isDraft ? input.draftState?.dirty ?? false : input.mcpDirty,
  };
}

type McpEnableToggleProps = {
  currentEnabled: boolean;
  isDraft: boolean;
  isEditableProfile: boolean;
  onDraftStateChange?: (next: { enabled?: boolean; dirty?: boolean }) => void;
  setMcpEnabled: (enabled: boolean) => void;
};

function McpEnableToggle({ currentEnabled, isDraft, isEditableProfile, onDraftStateChange, setMcpEnabled }: McpEnableToggleProps) {
  return (
    <div className="flex items-center justify-between rounded-md border p-3">
      <div className="space-y-1">
        <Label>Enable MCP</Label>
        <p className="text-xs text-muted-foreground">
          Allow this profile to use MCP servers during sessions.
        </p>
      </div>
      <Switch
        checked={currentEnabled}
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

function McpProfileHint({ isDraft, isEditableProfile }: { isDraft: boolean; isEditableProfile: boolean }) {
  if (isEditableProfile) return null;
  return (
    <p className="text-xs text-muted-foreground">
      {isDraft
        ? 'MCP config will be applied after the profile is saved.'
        : 'Save this profile to configure MCP servers.'}
    </p>
  );
}

export function ProfileMcpConfigCard({
  profileId,
  supportsMcp,
  initialConfig,
  draftState,
  onDraftStateChange,
  onToastError,
}: ProfileMcpConfigCardProps) {
  const {
    mcpEnabled, mcpServers, mcpError, mcpDirty, mcpStatus,
    setMcpEnabled, handleMcpServersChange, handleSaveMcp,
  } = useProfileMcpConfig({ profileId, supportsMcp, initialConfig, onToastError });

  if (!supportsMcp) return null;

  const state = resolveMcpConfigState({ draftState, profileId, mcpEnabled, mcpServers, mcpError, mcpDirty });

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div className="flex items-center gap-2">
          <CardTitle>MCP Configuration</CardTitle>
          {state.currentDirty && <UnsavedChangesBadge />}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <McpProfileHint isDraft={state.isDraft} isEditableProfile={state.isEditableProfile} />
        <McpEnableToggle
          currentEnabled={state.currentEnabled}
          isDraft={state.isDraft}
          isEditableProfile={state.isEditableProfile}
          onDraftStateChange={onDraftStateChange}
          setMcpEnabled={setMcpEnabled}
        />
        <McpServersEditor
          profileId={profileId}
          currentServers={state.currentServers}
          currentError={state.currentError}
          isDraft={state.isDraft}
          isEditableProfile={state.isEditableProfile}
          onDraftStateChange={onDraftStateChange}
          handleMcpServersChange={handleMcpServersChange}
        />
      </CardContent>
      <div className="flex justify-end px-6 pb-6">
        {state.isEditableProfile ? (
          <UnsavedSaveButton isDirty={state.currentDirty} isLoading={mcpStatus === 'loading'} status={mcpStatus} onClick={handleSaveMcp} />
        ) : null}
      </div>
    </Card>
  );
}
