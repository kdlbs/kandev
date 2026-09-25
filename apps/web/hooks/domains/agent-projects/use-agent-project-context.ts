"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  listAgentProjectContext,
  readAgentProjectContextFile,
  writeAgentProjectContextFile,
} from "@/lib/api/domains/agent-projects-api";
import type {
  AgentProjectContextEntry,
  AgentProjectContextFile,
} from "@/lib/types/http-agent-projects";

type ContextStatus = "loading" | "ready" | "error";

export function useAgentProjectContext(workspaceId: string, projectId: string) {
  const [entries, setEntries] = useState<AgentProjectContextEntry[]>([]);
  const [directory, setDirectory] = useState("");
  const [file, setFile] = useState<AgentProjectContextFile | null>(null);
  const [draft, setDraft] = useState("");
  const [status, setStatus] = useState<ContextStatus>("loading");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);
  const dirty = file?.content !== draft;

  const loadDirectory = useCallback(
    async (path: string) => {
      const request = ++generation.current;
      setStatus("loading");
      setError(null);
      setFile(null);
      try {
        const response = await listAgentProjectContext(workspaceId, projectId, path);
        if (request !== generation.current) return;
        setDirectory(path);
        setEntries(response.entries ?? []);
        setStatus("ready");
      } catch (caught) {
        if (request !== generation.current) return;
        setError(caught instanceof Error ? caught.message : String(caught));
        setStatus("error");
      }
    },
    [projectId, workspaceId],
  );

  useEffect(() => {
    void loadDirectory("");
    return () => {
      generation.current += 1;
    };
  }, [loadDirectory]);

  const openFile = useCallback(
    async (path: string) => {
      const request = ++generation.current;
      setBusy(true);
      setError(null);
      try {
        const response = await readAgentProjectContextFile(workspaceId, projectId, path);
        if (request !== generation.current) return;
        setFile(response);
        setDraft(response.content);
        setDirectory(path.split("/").slice(0, -1).join("/"));
        setStatus("ready");
      } catch (caught) {
        if (request !== generation.current) return;
        setError(caught instanceof Error ? caught.message : String(caught));
      } finally {
        if (request === generation.current) setBusy(false);
      }
    },
    [projectId, workspaceId],
  );

  const save = useCallback(async () => {
    if (!file || !dirty) return;
    setBusy(true);
    setError(null);
    try {
      const response = await writeAgentProjectContextFile(workspaceId, projectId, {
        path: file.path,
        content: draft,
        expectedHash: file.hash,
      });
      setFile({ ...file, content: draft, hash: response.hash });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally {
      setBusy(false);
    }
  }, [directory, dirty, draft, file, loadDirectory, projectId, workspaceId]);

  const goUp = useCallback(() => {
    const parentDirectory = file ? directory : directory.split("/").slice(0, -1).join("/");
    setFile(null);
    void loadDirectory(parentDirectory);
  }, [directory, file, loadDirectory]);

  return {
    entries,
    directory,
    file,
    draft,
    setDraft,
    status,
    busy,
    dirty,
    error,
    loadDirectory,
    openFile,
    save,
    goUp,
  };
}
