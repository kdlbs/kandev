'use client';

import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Label } from '@kandev/ui/label';
import { Switch } from '@kandev/ui/switch';
import { Textarea } from '@kandev/ui/textarea';
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

export function ProfileMcpConfigCard({
  profileId,
  supportsMcp,
  initialConfig,
  draftState,
  onDraftStateChange,
  onToastError,
}: ProfileMcpConfigCardProps) {
  const {
    mcpEnabled,
    mcpServers,
    mcpError,
    mcpDirty,
    mcpStatus,
    setMcpEnabled,
    handleMcpServersChange,
    handleSaveMcp,
  } = useProfileMcpConfig({
    profileId,
    supportsMcp,
    initialConfig,
    onToastError,
  });

  if (!supportsMcp) {
    return null;
  }

  const isDraft = Boolean(draftState);
  const isEditableProfile = !isDraft && Boolean(profileId) && !profileId.startsWith('draft-');

  const currentEnabled = isDraft ? draftState?.enabled ?? false : mcpEnabled;
  const currentServers = isDraft ? draftState?.servers ?? '' : mcpServers;
  const currentError = isDraft ? draftState?.error ?? null : mcpError;
  const currentDirty = isDraft ? draftState?.dirty ?? false : mcpDirty;

  const popularServers: Record<string, Record<string, unknown>> = {
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

  const applyPopularServer = (label: string) => {
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
    servers[label] = popularServers[label] ?? { type: 'stdio', command: 'npx', args: ['-y'] };
    root.mcpServers = servers;
    const nextValue = JSON.stringify(root, null, 2);

    if (isDraft) {
      onDraftStateChange?.({ servers: nextValue, dirty: true, error: null });
      return;
    }
    handleMcpServersChange(nextValue);
  };

  const handleDraftServersChange = (value: string) => {
    if (!onDraftStateChange) return;
    let error: string | null = null;
    if (value.trim()) {
      try {
        const parsed = JSON.parse(value);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          error = 'MCP servers config must be a JSON object';
        } else if ('mcpServers' in parsed) {
          const nested = (parsed as { mcpServers?: unknown }).mcpServers;
          if (!nested || typeof nested !== 'object' || Array.isArray(nested)) {
            error = 'mcpServers must be a JSON object';
          }
        }
      } catch {
        error = 'Invalid JSON';
      }
    }
    onDraftStateChange({ servers: value, dirty: true, error });
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div className="flex items-center gap-2">
          <CardTitle>MCP Configuration</CardTitle>
          {currentDirty && <UnsavedChangesBadge />}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {!isEditableProfile && (
          <p className="text-xs text-muted-foreground">
            {isDraft
              ? 'MCP config will be applied after the profile is saved.'
              : 'Save this profile to configure MCP servers.'}
          </p>
        )}
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

        <div className="space-y-2">
          <Label htmlFor={`mcp-servers-${profileId}`}>MCP servers (JSON)</Label>
          <Textarea
            id={`mcp-servers-${profileId}`}
            className="min-h-[200px] font-mono text-xs"
            value={currentServers}
            onChange={(event) => {
              if (isDraft) {
                handleDraftServersChange(event.target.value);
                return;
              }
              handleMcpServersChange(event.target.value);
            }}
            disabled={!isEditableProfile && !isDraft}
          />
          <p className="text-xs text-muted-foreground">
            MCP definitions are stored in the database and resolved per executor at runtime. This does not override your local agent config.</p>
          <p className="text-xs font-medium text-muted-foreground">Popular servers</p>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
              onClick={() => applyPopularServer('playwright')}
            >
              + Playwright MCP
            </button>
            <button
              type="button"
              className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
              onClick={() => applyPopularServer('chrome-devtools')}
            >
              + Chrome DevTools MCP
            </button>
            <button
              type="button"
              className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
              onClick={() => applyPopularServer('context7')}
            >
              + Context7 MCP
            </button>
            <button
              type="button"
              className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
              onClick={() => applyPopularServer('github')}
            >
              + GitHub MCP
            </button>
          </div>
          {currentError && <p className="text-sm text-destructive">{currentError}</p>}
        </div>
      </CardContent>
      <div className="flex justify-end px-6 pb-6">
        {isEditableProfile ? (
          <UnsavedSaveButton
            isDirty={currentDirty}
            isLoading={mcpStatus === 'loading'}
            status={mcpStatus}
            onClick={handleSaveMcp}
          />
        ) : null}
      </div>
    </Card>
  );
}
