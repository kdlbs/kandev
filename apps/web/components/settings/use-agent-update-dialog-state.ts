"use client";

import { useCallback, useEffect, useState } from "react";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

type UseAgentUpdateDialogStateOptions = {
  agentName: string;
  job?: AgentUpdateJob;
  onPreview: (agentName: string) => Promise<AgentUpdatePreview>;
  onUpdate: (agentName: string) => Promise<AgentUpdateJob>;
};

export function useAgentUpdateDialogState({
  agentName,
  job,
  onPreview,
  onUpdate,
}: UseAgentUpdateDialogStateOptions) {
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<AgentUpdatePreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [starting, setStarting] = useState(false);
  const [activeJobID, setActiveJobID] = useState<string | null>(null);
  const activeJob = activeJobID === job?.job_id ? job : undefined;

  const reset = useCallback(() => {
    setPreview(null);
    setPreviewError(null);
    setLoading(false);
    setStarting(false);
    setActiveJobID(null);
  }, []);

  const loadPreview = useCallback(async () => {
    setLoading(true);
    setPreviewError(null);
    try {
      setPreview(await onPreview(agentName));
    } catch (error) {
      setPreviewError(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [agentName, onPreview]);

  useEffect(() => {
    if (open) void loadPreview();
  }, [loadPreview, open]);

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) reset();
  };

  const approve = async () => {
    setStarting(true);
    try {
      const nextJob = await onUpdate(agentName);
      setActiveJobID(nextJob.job_id);
    } catch (error) {
      setPreviewError(errorMessage(error));
    } finally {
      setStarting(false);
    }
  };

  return {
    activeJob,
    approve,
    handleOpenChange,
    loading,
    loadPreview,
    open,
    preview,
    previewError,
    starting,
  };
}
