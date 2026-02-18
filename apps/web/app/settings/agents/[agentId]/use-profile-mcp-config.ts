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

const EMPTY_EXAMPLE = '{\n  "mcpServers": {}\n}';

function serializeServers(config: AgentProfileMcpConfig | null): string {
  if (!config?.servers || Object.keys(config.servers).length === 0) {
    return EMPTY_EXAMPLE;
  }
  return JSON.stringify({ mcpServers: config.servers }, null, 2);
}

function normalizeServers(value: unknown): Record<string, McpServerDef> {
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
}

function useResetOnUnsupported(
  supportsMcp: boolean,
  isEditableProfile: boolean,
  setters: {
    setMcpConfig: (v: AgentProfileMcpConfig | null) => void;
    setMcpEnabledState: (v: boolean) => void;
    setMcpServers: (v: string) => void;
    setMcpDirty: (v: boolean) => void;
    setMcpError: (v: string | null) => void;
    setMcpStatus: (v: McpStatus) => void;
  },
) {
  useEffect(() => {
    if (supportsMcp && isEditableProfile) return;
    let active = true;
    Promise.resolve().then(() => {
      if (!active) return;
      setters.setMcpConfig(null);
      setters.setMcpEnabledState(false);
      setters.setMcpServers(EMPTY_EXAMPLE);
      setters.setMcpDirty(false);
      setters.setMcpError(null);
      setters.setMcpStatus('idle');
    });
    return () => { active = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally only tracking these deps
  }, [supportsMcp, isEditableProfile]);
}

function useFetchConfig(
  profileId: string,
  supportsMcp: boolean,
  isEditableProfile: boolean,
  hasInitialConfig: boolean,
  setters: {
    setMcpConfig: (v: AgentProfileMcpConfig) => void;
    setMcpEnabledState: (v: boolean) => void;
    setMcpServers: (v: string) => void;
    setMcpDirty: (v: boolean) => void;
    setMcpError: (v: string | null) => void;
    setMcpStatus: (v: McpStatus) => void;
  },
) {
  useEffect(() => {
    if (!supportsMcp || !isEditableProfile || hasInitialConfig) return;
    let active = true;
    Promise.resolve().then(() => {
      if (!active) return;
      setters.setMcpStatus('loading');
    });
    getAgentProfileMcpConfigAction(profileId)
      .then((config) => {
        if (!active) return;
        setters.setMcpConfig(config);
        setters.setMcpEnabledState(config.enabled);
        setters.setMcpServers(serializeServers(config));
        setters.setMcpDirty(false);
        setters.setMcpError(null);
        setters.setMcpStatus('idle');
      })
      .catch((error) => {
        if (!active) return;
        setters.setMcpError(error instanceof Error ? error.message : 'Failed to load MCP config');
        setters.setMcpStatus('error');
      });

    return () => { active = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally only tracking these deps
  }, [profileId, supportsMcp, isEditableProfile, hasInitialConfig]);
}

export function useProfileMcpConfig({
  profileId,
  supportsMcp,
  initialConfig,
  onToastError,
}: UseProfileMcpConfigParams): UseProfileMcpConfigResult {
  const initialServers = serializeServers(initialConfig ?? null);
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

  const stateSetters = {
    setMcpConfig,
    setMcpEnabledState,
    setMcpServers,
    setMcpDirty,
    setMcpError,
    setMcpStatus,
  };

  useResetOnUnsupported(supportsMcp, isEditableProfile, stateSetters);
  useFetchConfig(profileId, supportsMcp, isEditableProfile, hasInitialConfig, stateSetters);

  const isEmptyExample = (value: string) => value.trim() === EMPTY_EXAMPLE.trim();

  const setMcpEnabled = (enabled: boolean) => {
    setMcpEnabledState(enabled);
    setMcpDirty(true);
  };

  const handleMcpServersChange = (value: string) => {
    if (!value.trim()) {
      setMcpServers(EMPTY_EXAMPLE);
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
    if (!isEditableProfile) return;
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
      setMcpServers(serializeServers(updated));
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
