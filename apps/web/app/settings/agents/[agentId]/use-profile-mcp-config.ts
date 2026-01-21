'use client';

import { useEffect, useState } from 'react';
import { getAgentProfileMcpConfigAction, updateAgentProfileMcpConfigAction } from '@/app/actions/agents';
import type { AgentProfileMcpConfig, McpServerDef } from '@/lib/types/http';

type McpStatus = 'idle' | 'loading' | 'success' | 'error';

type UseProfileMcpConfigParams = {
  profileId: string;
  supportsMcp: boolean;
  initialConfig?: AgentProfileMcpConfig | null;
  onToastError: (error: unknown) => void;
};

type UseProfileMcpConfigResult = {
  mcpEnabled: boolean;
  mcpServers: string;
  mcpError: string | null;
  mcpDirty: boolean;
  mcpStatus: McpStatus;
  setMcpEnabled: (enabled: boolean) => void;
  handleMcpServersChange: (value: string) => void;
  handleSaveMcp: () => Promise<void>;
};

export function useProfileMcpConfig({
  profileId,
  supportsMcp,
  initialConfig,
  onToastError,
}: UseProfileMcpConfigParams): UseProfileMcpConfigResult {
  const emptyExample = '{\n  "mcpServers": {}\n}';
  const initialServers =
    initialConfig?.servers && Object.keys(initialConfig.servers).length > 0
      ? JSON.stringify({ mcpServers: initialConfig.servers }, null, 2)
      : emptyExample;
  const [mcpConfig, setMcpConfig] = useState<AgentProfileMcpConfig | null>(
    initialConfig ?? null
  );
  const [mcpEnabled, setMcpEnabledState] = useState(initialConfig?.enabled ?? false);
  const [mcpServers, setMcpServers] = useState(initialServers);
  const [mcpError, setMcpError] = useState<string | null>(null);
  const [mcpDirty, setMcpDirty] = useState(false);
  const [mcpStatus, setMcpStatus] = useState<McpStatus>('idle');
  const [hasInitialConfig] = useState(initialConfig !== undefined);

  const isEditableProfile = Boolean(profileId) && !profileId.startsWith('draft-');

  useEffect(() => {
    let active = true;
    if (!supportsMcp || !isEditableProfile) {
      Promise.resolve().then(() => {
        if (!active) return;
        setMcpConfig(null);
        setMcpEnabledState(false);
        setMcpServers(emptyExample);
        setMcpDirty(false);
        setMcpError(null);
        setMcpStatus('idle');
      });
      return () => {
        active = false;
      };
    }
    if (hasInitialConfig) {
      return () => {
        active = false;
      };
    }

    Promise.resolve().then(() => {
      if (!active) return;
      setMcpStatus('loading');
    });
    getAgentProfileMcpConfigAction(profileId)
      .then((config) => {
        if (!active) return;
        setMcpConfig(config);
        setMcpEnabledState(config.enabled);
        const nextServers =
          config.servers && Object.keys(config.servers).length > 0
            ? JSON.stringify({ mcpServers: config.servers }, null, 2)
            : emptyExample;
        setMcpServers(nextServers);
        setMcpDirty(false);
        setMcpError(null);
        setMcpStatus('idle');
      })
      .catch((error) => {
        if (!active) return;
        setMcpError(error instanceof Error ? error.message : 'Failed to load MCP config');
        setMcpStatus('error');
      });

    return () => {
      active = false;
    };
  }, [profileId, supportsMcp, isEditableProfile, hasInitialConfig]);

  const isEmptyExample = (value: string) => value.trim() === emptyExample.trim();

  const setMcpEnabled = (enabled: boolean) => {
    setMcpEnabledState(enabled);
    setMcpDirty(true);
  };

  const normalizeServers = (value: unknown) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error('MCP servers config must be a JSON object');
    }
    if ('mcpServers' in value) {
      const nested = (value as { mcpServers?: unknown }).mcpServers;
      if (!nested || typeof nested !== 'object' || Array.isArray(nested)) {
        throw new Error('mcpServers must be a JSON object');
      }
      return nested as Record<string, McpServerDef>;
    }
    return value as Record<string, McpServerDef>;
  };

  const handleMcpServersChange = (value: string) => {
    if (!value.trim()) {
      setMcpServers(emptyExample);
    } else {
      setMcpServers(value);
    }
    setMcpDirty(true);
    if (!value.trim()) {
      setMcpError(null);
      return;
    }
    try {
      const parsed = JSON.parse(value);
      normalizeServers(parsed);
      setMcpError(null);
    } catch {
      setMcpError('Invalid JSON');
    }
  };

  const handleSaveMcp = async () => {
    if (!isEditableProfile) {
      return;
    }
    setMcpStatus('loading');
    let servers: Record<string, McpServerDef> = {};
    try {
      const raw = isEmptyExample(mcpServers) ? '{}' : mcpServers;
      const parsed = raw.trim() ? JSON.parse(raw) : {};
      servers = normalizeServers(parsed);
    } catch (error) {
      setMcpStatus('error');
      setMcpError(error instanceof Error ? error.message : 'Invalid MCP config');
      return;
    }

    try {
      const updated = await updateAgentProfileMcpConfigAction(profileId, {
        enabled: mcpEnabled,
        mcpServers: servers,
        meta: mcpConfig?.meta ?? {},
      });
      setMcpConfig(updated);
      setMcpEnabledState(updated.enabled);
      const nextServers =
        updated.servers && Object.keys(updated.servers).length > 0
          ? JSON.stringify({ mcpServers: updated.servers }, null, 2)
          : emptyExample;
      setMcpServers(nextServers);
      setMcpDirty(false);
      setMcpError(null);
      setMcpStatus('success');
    } catch (error) {
      setMcpStatus('error');
      onToastError(error);
    }
  };

  return {
    mcpEnabled,
    mcpServers,
    mcpError,
    mcpDirty,
    mcpStatus,
    setMcpEnabled,
    handleMcpServersChange,
    handleSaveMcp,
  };
}
