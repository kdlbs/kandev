"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "@/lib/toast/sonner";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import * as officeApi from "@/lib/api/domains/office-api";
import { InstructionFileList } from "./instruction-file-list";
import { InstructionEditor } from "./instruction-editor";
import { useTranslation } from "react-i18next";

export type InstructionFile = {
  id: string;
  filename: string;
  content: string;
  is_entry: boolean;
  created_at: string;
  updated_at: string;
};

type AgentInstructionsTabProps = {
  agent: AgentProfile;
};

export function AgentInstructionsTab({ agent }: AgentInstructionsTabProps) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<InstructionFile[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const fetchFiles = useCallback(async () => {
    try {
      const res = await officeApi.listInstructions(agent.id);
      const items = (res as { files?: InstructionFile[] }).files ?? [];
      setFiles(items);
      if (items.length > 0 && !selectedFile) {
        setSelectedFile(items[0].filename);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToLoadInstructions"));
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
        await officeApi.upsertInstruction(agent.id, filename, content);
        toast.success(t("office:saved", { filename }));
        await fetchFiles();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToSave"));
      }
    },
    [agent.id, fetchFiles],
  );

  const handleDelete = useCallback(
    async (filename: string) => {
      try {
        await officeApi.deleteInstruction(agent.id, filename);
        toast.success(t("office:deleted", { filename }));
        setSelectedFile(null);
        await fetchFiles();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToDelete"));
      }
    },
    [agent.id, fetchFiles],
  );

  const handleAddFile = useCallback(
    async (filename: string) => {
      try {
        await officeApi.upsertInstruction(agent.id, filename, "");
        await fetchFiles();
        setSelectedFile(filename);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToCreateFile"));
      }
    },
    [agent.id, fetchFiles],
  );

  const active = files.find((f) => f.filename === selectedFile) ?? null;

  if (isLoading) {
    return (
      <div className="mt-4 flex items-center justify-center py-12">
        <p className="text-sm text-muted-foreground">{t("office:loadingInstructions")}</p>
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
      <InstructionEditor
        file={active}
        siblingFilenames={files.map((f) => f.filename)}
        onSave={handleSave}
        onDelete={handleDelete}
      />
    </div>
  );
}
