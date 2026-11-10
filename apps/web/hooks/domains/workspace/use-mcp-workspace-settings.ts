"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  createMCPServer,
  deleteMCPServer,
  installMCPMarketplaceEntry,
  listMCPServers,
  refreshMCPMarketplace,
  searchMCPMarketplace,
  updateMCPServer,
} from "@/lib/api/domains/mcp-api";
import { useMCPRegistrationCompletion } from "./use-mcp-registration-completion";
import type {
  MCPDefinitionInput,
  MCPDefinitionPatch,
  MCPMarketplaceInstallInput,
  MCPMarketplaceSearchResponse,
  MCPServerDefinition,
} from "@/lib/types/http-mcp";

export function useMCPWorkspaceDefinitions(workspaceId: string | null | undefined) {
  const [definitions, setDefinitions] = useState<MCPServerDefinition[]>([]);
  const [loadedWorkspace, setLoadedWorkspace] = useState<string | null>(null);
  const loadedWorkspaceRef = useRef<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const requestRef = useRef(0);
  const workspaceRef = useRef(workspaceId);
  workspaceRef.current = workspaceId;

  const reload = useCallback(async () => {
    const requestID = ++requestRef.current;
    const isCurrent = () =>
      requestRef.current === requestID && workspaceRef.current === workspaceId;
    if (!workspaceId) return [];
    setLoading(loadedWorkspaceRef.current !== workspaceId);
    setRefreshing(loadedWorkspaceRef.current === workspaceId);
    try {
      const next = await listMCPServers(workspaceId, { cache: "no-store" });
      if (!isCurrent()) return [];
      loadedWorkspaceRef.current = workspaceId;
      setLoadedWorkspace(workspaceId);
      setDefinitions(next);
      setError(null);
      return next;
    } catch (cause) {
      if (isCurrent()) setError(cause);
      return [];
    } finally {
      if (isCurrent()) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, [workspaceId]);

  useEffect(() => {
    loadedWorkspaceRef.current = null;
    setLoadedWorkspace(null);
    setDefinitions([]);
    setError(null);
    setLoading(false);
    setRefreshing(false);
    void reload();
    return () => {
      requestRef.current += 1;
    };
  }, [reload]);

  return {
    definitions: loadedWorkspace === workspaceId ? definitions : [],
    loading,
    refreshing,
    error,
    reload,
  };
}

function useMCPWorkspaceCatalog(workspaceId: string) {
  const [servers, setServers] = useState<MCPServerDefinition[]>([]);
  const requestRef = useRef(0);
  const generationRef = useRef(0);
  const workspaceRef = useRef(workspaceId);
  workspaceRef.current = workspaceId;
  useEffect(() => {
    setServers([]);
    return () => {
      generationRef.current += 1;
      requestRef.current += 1;
    };
  }, [workspaceId]);
  const load = useCallback(async () => {
    const request = ++requestRef.current;
    const next = await listMCPServers(workspaceId, { cache: "no-store" });
    if (request !== requestRef.current || workspaceRef.current !== workspaceId) return false;
    setServers(next);
    return true;
  }, [workspaceId]);
  const reconcile = useCallback((created: MCPServerDefinition) => {
    requestRef.current += 1;
    setServers((current) =>
      current.some((server) => server.id === created.id)
        ? current.map((server) => (server.id === created.id ? created : server))
        : [...current, created],
    );
  }, []);
  const update = useCallback(
    async (id: string, payload: MCPDefinitionPatch) => {
      const generation = generationRef.current;
      const updated = await updateMCPServer(workspaceId, id, payload);
      if (generation === generationRef.current && workspaceRef.current === workspaceId)
        reconcile(updated);
      return updated;
    },
    [workspaceId, reconcile],
  );
  const remove = useCallback(
    async (server: MCPServerDefinition, confirm = false) => {
      const generation = generationRef.current;
      await deleteMCPServer(workspaceId, server.id, server.revision, confirm);
      if (generation === generationRef.current && workspaceRef.current === workspaceId) {
        requestRef.current += 1;
        setServers((current) => current.filter((item) => item.id !== server.id));
      }
    },
    [workspaceId],
  );
  return { servers, load, reconcile, update, remove };
}

export function useMCPWorkspaceSettings(workspaceId: string) {
  const catalog = useMCPWorkspaceCatalog(workspaceId);
  const registration = useMCPRegistrationCompletion(workspaceId);
  const [marketplace, setMarketplace] = useState<MCPMarketplaceSearchResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [marketplaceLoading, setMarketplaceLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const marketplaceRequestRef = useRef(0);
  const reloadRequestRef = useRef(0);
  const workspaceRef = useRef(workspaceId);
  workspaceRef.current = workspaceId;
  const searchMarketplace = useCallback(async (query = "") => {
    const requestID = ++marketplaceRequestRef.current;
    setMarketplaceLoading(true);
    try {
      const next = await searchMCPMarketplace(query);
      if (marketplaceRequestRef.current === requestID) setMarketplace(next);
      return next;
    } finally {
      if (marketplaceRequestRef.current === requestID) setMarketplaceLoading(false);
    }
  }, []);
  const reload = useCallback(async () => {
    const requestID = ++reloadRequestRef.current;
    const isCurrent = () =>
      reloadRequestRef.current === requestID && workspaceRef.current === workspaceId;
    setLoading(true);
    try {
      await Promise.all([catalog.load(), searchMarketplace()]);
      if (isCurrent()) setError(null);
    } catch (cause) {
      if (isCurrent()) setError(cause);
    } finally {
      if (isCurrent()) setLoading(false);
    }
  }, [catalog.load, workspaceId, searchMarketplace]);
  useEffect(() => {
    setError(null);
    void reload();
    return () => {
      reloadRequestRef.current += 1;
      marketplaceRequestRef.current += 1;
    };
  }, [reload]);
  const register = async (operation: () => Promise<MCPServerDefinition>) => {
    const result = await registration.register(operation);
    if (result.status === "saved") catalog.reconcile(result.definition);
    return result;
  };
  const reviewRegistration = async () => {
    const generation = reloadRequestRef.current;
    const isCurrent = () =>
      generation === reloadRequestRef.current && workspaceRef.current === workspaceId;
    try {
      if ((await catalog.load()) && isCurrent()) {
        registration.reviewed();
        setError(null);
      }
    } catch (cause) {
      if (isCurrent()) setError(cause);
    }
  };
  return {
    servers: catalog.servers,
    marketplace,
    loading,
    marketplaceLoading,
    error,
    reload,
    searchMarketplace,
    registration,
    reviewRegistration,
    create: (payload: MCPDefinitionInput) => register(() => createMCPServer(workspaceId, payload)),
    install: (payload: MCPMarketplaceInstallInput) =>
      register(() => installMCPMarketplaceEntry(workspaceId, payload)),
    update: catalog.update,
    remove: catalog.remove,
    refresh: async () => {
      await refreshMCPMarketplace();
      return searchMarketplace();
    },
  };
}
