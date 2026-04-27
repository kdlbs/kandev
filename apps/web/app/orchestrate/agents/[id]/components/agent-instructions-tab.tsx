"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import { InstructionFileList } from "./instruction-file-list";
import { InstructionEditor } from "./instruction-editor";

export type InstructionFile = {
  id: string;
  filename: string;
  content: string;
  is_entry: boolean;
  created_at: string;
  updated_at: string;
};

type AgentInstructionsTabProps = {
  agent: AgentInstance;
};

export function AgentInstructionsTab({ agent }: AgentInstructionsTabProps) {
  const [files, setFiles] = useState<InstructionFile[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const fetchFiles = useCallback(async () => {
    try {
      const res = await orchestrateApi.listInstructions(agent.id);
      const items = (res as { files?: InstructionFile[] }).files ?? [];
      setFiles(items);
      if (items.length > 0 && !selectedFile) {
        setSelectedFile(items[0].filename);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load instructions");
    } finally {
      setIsLoading(false);
    }
  }, [agent.id, selectedFile]);

  useEffect(() => {
    fetchFiles();
  }, [fetchFiles]);

  const handleSave = useCallback(
    async (filename: string, content: string) => {
      try {
        await orchestrateApi.upsertInstruction(agent.id, filename, content);
        toast.success(`Saved ${filename}`);
        await fetchFiles();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to save");
      }
    },
    [agent.id, fetchFiles],
  );

  const handleDelete = useCallback(
    async (filename: string) => {
      try {
        await orchestrateApi.deleteInstruction(agent.id, filename);
        toast.success(`Deleted ${filename}`);
        setSelectedFile(null);
        await fetchFiles();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to delete");
      }
    },
    [agent.id, fetchFiles],
  );

  const handleAddFile = useCallback(
    async (filename: string) => {
      try {
        await orchestrateApi.upsertInstruction(agent.id, filename, "");
        await fetchFiles();
        setSelectedFile(filename);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to create file");
      }
    },
    [agent.id, fetchFiles],
  );

  const active = files.find((f) => f.filename === selectedFile) ?? null;

  if (isLoading) {
    return (
      <div className="mt-4 flex items-center justify-center py-12">
        <p className="text-sm text-muted-foreground">Loading instructions...</p>
      </div>
    );
  }

  return (
    <div className="mt-4 flex gap-4 min-h-[500px]">
      <InstructionFileList
        files={files}
        selectedFile={selectedFile}
        onSelect={setSelectedFile}
        onAdd={handleAddFile}
      />
      <InstructionEditor file={active} onSave={handleSave} onDelete={handleDelete} />
    </div>
  );
}
