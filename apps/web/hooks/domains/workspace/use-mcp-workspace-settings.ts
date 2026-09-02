"use client";

import { useCallback, useEffect, useState } from "react";
import {
  createMCPServer,
  deleteMCPServer,
  installMCPMarketplaceEntry,
  listMCPServers,
  refreshMCPMarketplace,
  searchMCPMarketplace,
  updateMCPServer,
} from "@/lib/api/domains/mcp-api";
import type {
  MCPDefinitionInput,
  MCPDefinitionPatch,
  MCPMarketplaceInstallInput,
  MCPMarketplaceSearchResponse,
  MCPServerDefinition,
} from "@/lib/types/http-mcp";

export function useMCPWorkspaceDefinitions(workspaceId: string | null | undefined) {
  const [definitions, setDefinitions] = useState<MCPServerDefinition[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const reload = useCallback(async () => {
    if (!workspaceId) {
      setDefinitions([]);
      setError(null);
      return [];
    }
    setLoading(true);
    try {
      const next = await listMCPServers(workspaceId, { cache: "no-store" });
      setDefinitions(next);
      setError(null);
      return next;
    } catch (cause) {
      setError(cause);
      return [];
    } finally {
      setLoading(false);
    }
  }, [workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { definitions, loading, error, reload };
}

export function useMCPWorkspaceSettings(workspaceId: string) {
  const [servers, setServers] = useState<MCPServerDefinition[]>([]);
  const [marketplace, setMarketplace] = useState<MCPMarketplaceSearchResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [marketplaceLoading, setMarketplaceLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const loadServers = useCallback(async () => {
    const next = await listMCPServers(workspaceId, { cache: "no-store" });
    setServers(next);
    return next;
  }, [workspaceId]);

  const searchMarketplace = useCallback(async (query = "") => {
    setMarketplaceLoading(true);
    try {
      const next = await searchMCPMarketplace(query);
      setMarketplace(next);
      setError(null);
      return next;
    } finally {
      setMarketplaceLoading(false);
    }
  }, []);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      await Promise.all([loadServers(), searchMarketplace()]);
      setError(null);
    } catch (cause) {
      setError(cause);
    } finally {
      setLoading(false);
    }
  }, [loadServers, searchMarketplace]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const create = useCallback(
    async (payload: MCPDefinitionInput) => {
      const created = await createMCPServer(workspaceId, payload);
      setServers((current) => [...current, created]);
      return created;
    },
    [workspaceId],
  );

  const update = useCallback(
    async (serverId: string, payload: MCPDefinitionPatch) => {
      const updated = await updateMCPServer(workspaceId, serverId, payload);
      setServers((current) => current.map((server) => (server.id === serverId ? updated : server)));
      return updated;
    },
    [workspaceId],
  );

  const remove = useCallback(
    async (server: MCPServerDefinition, confirm = false) => {
      await deleteMCPServer(workspaceId, server.id, server.revision, confirm);
      setServers((current) => current.filter((item) => item.id !== server.id));
    },
    [workspaceId],
  );

  const install = useCallback(
    async (payload: MCPMarketplaceInstallInput) => {
      const created = await installMCPMarketplaceEntry(workspaceId, payload);
      setServers((current) => [...current, created]);
      return created;
    },
    [workspaceId],
  );

  const refresh = useCallback(async () => {
    await refreshMCPMarketplace();
    return searchMarketplace();
  }, [searchMarketplace]);

  return {
    servers,
    marketplace,
    loading,
    marketplaceLoading,
    error,
    reload,
    searchMarketplace,
    create,
    update,
    remove,
    install,
    refresh,
  };
}
