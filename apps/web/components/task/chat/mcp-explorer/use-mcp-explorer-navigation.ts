"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { selectMcpServerName, selectMcpToolName } from "./mcp-explorer-view-model";

export type McpExplorerPage = "servers" | "tools" | "tool";

export function useMcpExplorerNavigation({
  servers,
  open,
  touch,
}: {
  servers: MCPAttachmentServer[];
  open: boolean;
  touch: boolean;
}) {
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [selectedToolName, setSelectedToolName] = useState<string | null>(null);
  const [page, setPage] = useState<McpExplorerPage>(touch ? "servers" : "tools");
  const listScrollRef = useRef<HTMLDivElement>(null);
  const listOffsetsRef = useRef(new Map<string, number>());
  const restoreToolRef = useRef<string | null>(null);
  const wasOpenRef = useRef(false);
  const currentName = useMemo(
    () => selectMcpServerName(servers, selectedName),
    [selectedName, servers],
  );
  const server = servers.find((candidate) => candidate.name === currentName) ?? null;
  const toolName = selectMcpToolName(server, selectedToolName);
  const tool = server?.tools?.find((candidate) => candidate.name === toolName) ?? null;

  useEffect(() => {
    const nextName = selectMcpServerName(servers, selectedName);
    if (selectedName && nextName !== selectedName) {
      restoreToolRef.current = null;
      setSelectedToolName(null);
      setPage("tools");
    }
    setSelectedName(nextName);
  }, [selectedName, servers]);

  useEffect(() => {
    if (!open) {
      wasOpenRef.current = false;
      return;
    }
    if (wasOpenRef.current) return;
    wasOpenRef.current = true;
    listOffsetsRef.current.clear();
    setSelectedName(selectMcpServerName(servers));
    setSelectedToolName(null);
    setPage(touch ? "servers" : "tools");
  }, [open, servers, touch]);

  useEffect(() => {
    if (!selectedToolName || toolName) return;
    restoreToolRef.current = null;
    setSelectedToolName(null);
    setPage("tools");
  }, [selectedToolName, toolName]);

  useLayoutEffect(() => {
    if (page !== "tools" || !server || !listScrollRef.current) return;
    listScrollRef.current.scrollTop = listOffsetsRef.current.get(server.name) ?? 0;
    const restoreName = restoreToolRef.current;
    if (!restoreName) return;
    const rows =
      listScrollRef.current.querySelectorAll<HTMLButtonElement>("button[data-tool-name]");
    Array.from(rows)
      .find((row) => row.dataset.toolName === restoreName)
      ?.focus();
    restoreToolRef.current = null;
  }, [page, server]);

  const selectServer = (name: string) => {
    if (server && listScrollRef.current) {
      listOffsetsRef.current.set(server.name, listScrollRef.current.scrollTop);
    }
    setSelectedName(name);
    setSelectedToolName(null);
    setPage("tools");
  };

  const selectTool = (name: string) => {
    if (server && listScrollRef.current) {
      listOffsetsRef.current.set(server.name, listScrollRef.current.scrollTop);
    }
    setSelectedToolName(name);
    setPage("tool");
  };

  const backToTools = () => {
    restoreToolRef.current = selectedToolName;
    setPage("tools");
  };

  return {
    page,
    server,
    tool,
    selectedName: currentName,
    listScrollRef,
    selectServer,
    selectTool,
    backToTools,
    backToServers: () => setPage("servers"),
  };
}
