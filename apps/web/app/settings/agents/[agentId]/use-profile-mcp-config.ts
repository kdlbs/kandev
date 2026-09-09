"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Dispatch, MutableRefObject, SetStateAction } from "react";
import {
  getAgentProfileMcpConfigAction,
  updateAgentProfileMcpConfigAction,
} from "@/app/actions/agents";
import { t } from "@/lib/i18n";
import { useWebSocketClient } from "@/lib/ws/connection";
import type { AgentProfileMcpConfig, McpServerDef } from "@/lib/types/http";

type McpStatus = "idle" | "loading" | "success" | "error";

type UseProfileMcpConfigParams = {
  profileId: string;
  supportsMcp: boolean;
  initialConfig?: AgentProfileMcpConfig | null;
  onToastError: (error: unknown) => void;
};

type UseProfileMcpConfigResult = {
  mcpEnabled: boolean;
  mcpServers: string;
  mcpBaselineEnabled: boolean;
  mcpBaselineServers: string;
  mcpError: string | null;
  mcpConflict: boolean;
  mcpDirty: boolean;
  mcpStatus: McpStatus;
  setMcpEnabled: (enabled: boolean) => void;
  handleMcpServersChange: (value: string) => void;
  handleSaveMcp: () => Promise<void>;
  resetMcpDraft: () => void;
};

const EMPTY_EXAMPLE = '{\n  "mcpServers": {}\n}';
// The JSON key the editor validates against — an identifier, interpolated into
// the error messages below rather than written into the catalog.
const MCP_SERVERS_KEY = "mcpServers";
const isEmptyExample = (value: string) => value.trim() === EMPTY_EXAMPLE.trim();

type McpStateSetters = {
  setMcpConfig: (value: AgentProfileMcpConfig | null) => void;
  setMcpEnabledState: Dispatch<SetStateAction<boolean>>;
  setMcpServers: Dispatch<SetStateAction<string>>;
  setMcpDirty: (value: boolean) => void;
  setMcpConflict: (value: boolean) => void;
  setMcpError: (value: string | null) => void;
  setMcpStatus: (value: McpStatus) => void;
};

function serializeServers(config: AgentProfileMcpConfig | null): string {
  if (!config?.servers || Object.keys(config.servers).length === 0) {
    return EMPTY_EXAMPLE;
  }
  return JSON.stringify({ mcpServers: config.servers }, null, 2);
}

function normalizeServers(value: unknown): Record<string, McpServerDef> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(t("agents:mcpConfigMustBeJsonObject"));
  }
  if (MCP_SERVERS_KEY in value) {
    const nested = (value as { mcpServers?: unknown }).mcpServers;
    if (!nested || typeof nested !== "object" || Array.isArray(nested)) {
      throw new Error(t("agents:mcpKeyMustBeJsonObject", { key: MCP_SERVERS_KEY }));
    }
    return nested as Record<string, McpServerDef>;
  }
  return value as Record<string, McpServerDef>;
}

function useResetOnUnsupported(
  supportsMcp: boolean,
  isEditableProfile: boolean,
  setters: McpStateSetters,
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
      setters.setMcpConflict(false);
      setters.setMcpError(null);
      setters.setMcpStatus("idle");
    });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally only tracking these deps
  }, [supportsMcp, isEditableProfile]);
}

function useLatestMcpDraft(enabled: boolean, servers: string) {
  const ref = useRef({ enabled, servers });
  ref.current = { enabled, servers };
  return ref;
}

function resetMcpDraftState(config: AgentProfileMcpConfig | null, setters: McpStateSetters) {
  setters.setMcpEnabledState(config?.enabled ?? false);
  setters.setMcpServers(serializeServers(config));
  setters.setMcpDirty(false);
  setters.setMcpConflict(false);
  setters.setMcpError(null);
  setters.setMcpStatus("idle");
}

function mcpBaseline(config: AgentProfileMcpConfig | null) {
  return {
    mcpBaselineEnabled: config?.enabled ?? false,
    mcpBaselineServers: serializeServers(config),
  };
}

type McpConfigLoaderParams = {
  profileId: string;
  supportsMcp: boolean;
  isEditableProfile: boolean;
  hasInitialConfig: boolean;
  latestDraftRef: MutableRefObject<{ enabled: boolean; servers: string }>;
  requestGenerationRef: MutableRefObject<number>;
  baselineRef: MutableRefObject<AgentProfileMcpConfig | null>;
  conflictRef: MutableRefObject<boolean>;
  setters: McpStateSetters;
};

function useMcpConfigLoader({
  profileId,
  supportsMcp,
  isEditableProfile,
  hasInitialConfig,
  latestDraftRef,
  requestGenerationRef,
  baselineRef,
  conflictRef,
  setters,
}: McpConfigLoaderParams) {
  const loadConfig = useCallback(() => {
    if (!supportsMcp || !isEditableProfile) return;
    const generation = ++requestGenerationRef.current;
    setters.setMcpStatus("loading");
    getAgentProfileMcpConfigAction(profileId)
      .then((config) => {
        if (generation !== requestGenerationRef.current) return;
        const currentBaseline = baselineRef.current;
        const draftWasClean =
          !conflictRef.current &&
          latestDraftRef.current.enabled === (currentBaseline?.enabled ?? false) &&
          latestDraftRef.current.servers === serializeServers(currentBaseline);
        const draftMatchesIncoming =
          latestDraftRef.current.enabled === config.enabled &&
          latestDraftRef.current.servers === serializeServers(config);
        setters.setMcpConfig(config);
        if (draftWasClean || draftMatchesIncoming) {
          setters.setMcpEnabledState(config.enabled);
          setters.setMcpServers(serializeServers(config));
          setters.setMcpDirty(false);
          setters.setMcpConflict(false);
        } else {
          setters.setMcpDirty(true);
          setters.setMcpConflict(true);
        }
        setters.setMcpError(null);
        setters.setMcpStatus("idle");
      })
      .catch((error) => {
        if (generation !== requestGenerationRef.current) return;
        setters.setMcpError(
          error instanceof Error ? error.message : t("agents:failedToLoadMcpConfig"),
        );
        setters.setMcpStatus("error");
      });
  }, [
    baselineRef,
    conflictRef,
    isEditableProfile,
    latestDraftRef,
    profileId,
    requestGenerationRef,
    setters,
    supportsMcp,
  ]);

  useEffect(() => {
    if (hasInitialConfig) return;
    loadConfig();
    return () => {
      requestGenerationRef.current += 1;
    };
  }, [hasInitialConfig, loadConfig, requestGenerationRef]);

  return loadConfig;
}

type McpSaveParams = {
  profileId: string;
  isEditableProfile: boolean;
  mcpConflict: boolean;
  mcpEnabled: boolean;
  mcpServers: string;
  mcpConfig: AgentProfileMcpConfig | null;
  latestDraftRef: MutableRefObject<{ enabled: boolean; servers: string }>;
  requestGenerationRef: MutableRefObject<number>;
  onToastError: (error: unknown) => void;
  setters: McpStateSetters;
};

function useMcpSave({
  profileId,
  isEditableProfile,
  mcpConflict,
  mcpEnabled,
  mcpServers,
  mcpConfig,
  latestDraftRef,
  requestGenerationRef,
  onToastError,
  setters,
}: McpSaveParams) {
  return useCallback(async () => {
    if (!isEditableProfile || mcpConflict) return;
    requestGenerationRef.current += 1;
    const submittedEnabled = mcpEnabled;
    const submittedServers = mcpServers;
    setters.setMcpStatus("loading");

    let servers: Record<string, McpServerDef> = {};
    try {
      const raw = isEmptyExample(mcpServers) ? "{}" : mcpServers;
      const parsed = raw.trim() ? JSON.parse(raw) : {};
      servers = normalizeServers(parsed);
    } catch (error) {
      setters.setMcpStatus("error");
      setters.setMcpError(error instanceof Error ? error.message : t("agents:invalidMcpConfig"));
      throw error;
    }

    try {
      const updated = await updateAgentProfileMcpConfigAction(profileId, {
        enabled: mcpEnabled,
        mcpServers: servers,
        meta: mcpConfig?.meta ?? {},
      });
      setters.setMcpConfig(updated);
      setters.setMcpEnabledState((current) =>
        current === submittedEnabled ? updated.enabled : current,
      );
      setters.setMcpServers((current) =>
        current === submittedServers ? serializeServers(updated) : current,
      );
      setters.setMcpDirty(
        latestDraftRef.current.enabled !== submittedEnabled ||
          latestDraftRef.current.servers !== submittedServers,
      );
      setters.setMcpConflict(false);
      setters.setMcpError(null);
      setters.setMcpStatus("success");
    } catch (error) {
      setters.setMcpStatus("error");
      onToastError(error);
      throw error;
    }
  }, [
    isEditableProfile,
    latestDraftRef,
    mcpConfig?.meta,
    mcpConflict,
    mcpEnabled,
    mcpServers,
    onToastError,
    profileId,
    requestGenerationRef,
    setters,
  ]);
}

function updateMcpServersDraft(value: string, setters: McpStateSetters) {
  setters.setMcpServers(value.trim() ? value : EMPTY_EXAMPLE);
  setters.setMcpDirty(true);
  if (!value.trim()) {
    setters.setMcpError(null);
    return;
  }
  try {
    normalizeServers(JSON.parse(value));
    setters.setMcpError(null);
  } catch {
    setters.setMcpError(t("agents:invalidJson"));
  }
}

export function useProfileMcpConfig({
  profileId,
  supportsMcp,
  initialConfig,
  onToastError,
}: UseProfileMcpConfigParams): UseProfileMcpConfigResult {
  const websocketClient = useWebSocketClient();
  const initialServers = serializeServers(initialConfig ?? null);
  const [mcpConfig, setMcpConfig] = useState<AgentProfileMcpConfig | null>(initialConfig ?? null);
  const [mcpEnabled, setMcpEnabledState] = useState(initialConfig?.enabled ?? false);
  const [mcpServers, setMcpServers] = useState(initialServers);
  const [mcpError, setMcpError] = useState<string | null>(null);
  const [mcpConflict, setMcpConflict] = useState(false);
  const [mcpDirty, setMcpDirty] = useState(false);
  const [mcpStatus, setMcpStatus] = useState<McpStatus>("idle");
  const [hasInitialConfig] = useState(initialConfig !== undefined);
  const latestDraftRef = useLatestMcpDraft(mcpEnabled, mcpServers);
  const requestGenerationRef = useRef(0);
  const baselineRef = useRef(mcpConfig);
  const conflictRef = useRef(mcpConflict);
  baselineRef.current = mcpConfig;
  conflictRef.current = mcpConflict;

  const isEditableProfile = Boolean(profileId) && !profileId.startsWith("draft-");

  const stateSetters = useMemo(
    () => ({
      setMcpConfig,
      setMcpEnabledState,
      setMcpServers,
      setMcpDirty,
      setMcpConflict,
      setMcpError,
      setMcpStatus,
    }),
    [],
  );

  useResetOnUnsupported(supportsMcp, isEditableProfile, stateSetters);

  const loadConfig = useMcpConfigLoader({
    profileId,
    supportsMcp,
    isEditableProfile,
    hasInitialConfig,
    latestDraftRef,
    requestGenerationRef,
    baselineRef,
    conflictRef,
    setters: stateSetters,
  });

  useEffect(() => {
    if (!websocketClient || !supportsMcp || !isEditableProfile) return;
    return websocketClient.on("agent.profile.mcp_config.updated", (message) => {
      if (message.payload.profile_id !== profileId) return;
      loadConfig();
    });
  }, [isEditableProfile, loadConfig, profileId, supportsMcp, websocketClient]);

  const setMcpEnabled = (enabled: boolean) => {
    setMcpEnabledState(enabled);
    setMcpDirty(true);
  };

  const handleMcpServersChange = (value: string) => updateMcpServersDraft(value, stateSetters);

  const handleSaveMcp = useMcpSave({
    profileId,
    isEditableProfile,
    mcpConflict,
    mcpEnabled,
    mcpServers,
    mcpConfig,
    latestDraftRef,
    requestGenerationRef,
    onToastError,
    setters: stateSetters,
  });

  return {
    mcpEnabled,
    mcpServers,
    ...mcpBaseline(mcpConfig),
    mcpError,
    mcpConflict,
    mcpDirty,
    mcpStatus,
    setMcpEnabled,
    handleMcpServersChange,
    handleSaveMcp,
    resetMcpDraft: () => resetMcpDraftState(mcpConfig, stateSetters),
  };
}
