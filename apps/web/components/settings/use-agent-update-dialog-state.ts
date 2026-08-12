"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

type UseAgentUpdateDialogStateOptions = {
  agentName: string;
  job?: AgentUpdateJob;
  onPreview: (agentName: string, targetVersion?: string) => Promise<AgentUpdatePreview>;
  onUpdate: (agentName: string, targetVersion: string) => Promise<AgentUpdateJob>;
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
  const [approveError, setApproveError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [starting, setStarting] = useState(false);
  const [selectedTarget, setSelectedTarget] = useState("");
  const [activeJobID, setActiveJobID] = useState<string | null>(null);
  const previewRequestID = useRef(0);
  const activeJob = activeJobID === job?.job_id ? job : undefined;

  const reset = useCallback(() => {
    previewRequestID.current += 1;
    setPreview(null);
    setPreviewError(null);
    setApproveError(null);
    setLoading(false);
    setStarting(false);
    setSelectedTarget("");
    setActiveJobID(null);
  }, []);

  const loadPreview = useCallback(
    async (targetVersion?: string) => {
      const requestID = ++previewRequestID.current;
      setLoading(true);
      setPreview(null);
      setPreviewError(null);
      setApproveError(null);
      try {
        const nextPreview = await onPreview(agentName, targetVersion);
        if (requestID === previewRequestID.current) {
          setPreview(nextPreview);
          setSelectedTarget(nextPreview.target_version);
        }
      } catch (error) {
        if (requestID === previewRequestID.current) setPreviewError(errorMessage(error));
      } finally {
        if (requestID === previewRequestID.current) setLoading(false);
      }
    },
    [agentName, onPreview],
  );

  const selectTarget = useCallback(
    (targetVersion: string) => {
      setSelectedTarget(targetVersion);
      void loadPreview(targetVersion);
    },
    [loadPreview],
  );

  useEffect(() => {
    if (open) void loadPreview();
  }, [loadPreview, open]);

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) reset();
  };

  const approve = async () => {
    const requestID = previewRequestID.current;
    const targetVersion = selectedTarget || preview?.target_version || "";
    if (!targetVersion) return;
    setStarting(true);
    setApproveError(null);
    try {
      const nextJob = await onUpdate(agentName, targetVersion);
      if (requestID === previewRequestID.current) setActiveJobID(nextJob.job_id);
    } catch (error) {
      if (requestID === previewRequestID.current) setApproveError(errorMessage(error));
    } finally {
      if (requestID === previewRequestID.current) setStarting(false);
    }
  };

  return {
    activeJob,
    approve,
    approveError,
    handleOpenChange,
    loading,
    loadPreview,
    open,
    preview,
    previewError,
    selectTarget,
    selectedTarget,
    starting,
  };
}
