"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

type UseAgentUpdateDialogStateOptions = {
  agentName: string;
  job?: AgentUpdateJob;
  onPreview: (
    agentName: string,
    targetVersion?: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdatePreview>;
  onUpdate: (
    agentName: string,
    targetVersion: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdateJob>;
};

// i18n-exempt: internal selector token, never rendered as user-facing copy.
export const DEFAULT_RUNTIME_TARGET = "__kandev_default__";

function resetDialogState({
  previewRequestID,
  setPreview,
  setPreviewError,
  setApproveError,
  setLoading,
  setStarting,
  setSelectedTarget,
  setSelectedUseDefault,
  setActiveJobID,
}: {
  previewRequestID: { current: number };
  setPreview: (value: AgentUpdatePreview | null) => void;
  setPreviewError: (value: string | null) => void;
  setApproveError: (value: string | null) => void;
  setLoading: (value: boolean) => void;
  setStarting: (value: boolean) => void;
  setSelectedTarget: (value: string) => void;
  setSelectedUseDefault: (value: boolean) => void;
  setActiveJobID: (value: string | null) => void;
}) {
  previewRequestID.current += 1;
  setPreview(null);
  setPreviewError(null);
  setApproveError(null);
  setLoading(false);
  setStarting(false);
  setSelectedTarget("");
  setSelectedUseDefault(false);
  setActiveJobID(null);
}

async function approveRuntimeUpdate({
  requestID,
  previewRequestID,
  agentName,
  preview,
  selectedTarget,
  selectedUseDefault,
  onUpdate,
  setStarting,
  setApproveError,
  setActiveJobID,
}: {
  requestID: number;
  previewRequestID: { current: number };
  agentName: string;
  preview: AgentUpdatePreview | null;
  selectedTarget: string;
  selectedUseDefault: boolean;
  onUpdate: UseAgentUpdateDialogStateOptions["onUpdate"];
  setStarting: (value: boolean) => void;
  setApproveError: (value: string | null) => void;
  setActiveJobID: (value: string | null) => void;
}) {
  const targetVersion = selectedUseDefault
    ? preview?.default_version || preview?.target_version || ""
    : selectedTarget || preview?.target_version || "";
  if (!targetVersion) return;
  setStarting(true);
  setApproveError(null);
  try {
    const nextJob = selectedUseDefault
      ? await onUpdate(agentName, targetVersion, true)
      : await onUpdate(agentName, targetVersion);
    if (requestID === previewRequestID.current) setActiveJobID(nextJob.job_id);
  } catch (error) {
    if (requestID === previewRequestID.current) setApproveError(errorMessage(error));
  } finally {
    if (requestID === previewRequestID.current) setStarting(false);
  }
}

function handleDialogOpenChange(
  nextOpen: boolean,
  setOpen: (value: boolean) => void,
  reset: () => void,
) {
  setOpen(nextOpen);
  if (!nextOpen) reset();
}

type PreviewLoader = (targetVersion?: string, useDefault?: boolean) => Promise<void>;

function useRuntimeTargetSelectors(
  loadPreview: PreviewLoader,
  setActiveJobID: (value: string | null) => void,
  setSelectedTarget: (value: string) => void,
  setSelectedUseDefault: (value: boolean) => void,
) {
  const selectTarget = useCallback(
    (targetVersion: string) => {
      setActiveJobID(null);
      setSelectedTarget(targetVersion);
      setSelectedUseDefault(false);
      void loadPreview(targetVersion);
    },
    [loadPreview],
  );
  const selectDefault = useCallback(() => {
    setActiveJobID(null);
    setSelectedTarget(DEFAULT_RUNTIME_TARGET);
    setSelectedUseDefault(true);
    void loadPreview(undefined, true);
  }, [loadPreview]);
  return { selectTarget, selectDefault };
}

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
  const [selectedUseDefault, setSelectedUseDefault] = useState(false);
  const [activeJobID, setActiveJobID] = useState<string | null>(null);
  const previewRequestID = useRef(0);
  const activeJob = activeJobID === job?.job_id ? job : undefined;

  const reset = useCallback(() => {
    resetDialogState({
      previewRequestID,
      setPreview,
      setPreviewError,
      setApproveError,
      setLoading,
      setStarting,
      setSelectedTarget,
      setSelectedUseDefault,
      setActiveJobID,
    });
  }, []);

  const loadPreview = useCallback(
    async (targetVersion?: string, useDefault = false) => {
      const requestID = ++previewRequestID.current;
      setLoading(true);
      setPreviewError(null);
      setApproveError(null);
      try {
        const nextPreview = useDefault
          ? await onPreview(agentName, undefined, true)
          : await onPreview(agentName, targetVersion);
        if (requestID === previewRequestID.current) {
          setPreview(nextPreview);
          setSelectedTarget(useDefault ? DEFAULT_RUNTIME_TARGET : nextPreview.target_version);
          setSelectedUseDefault(useDefault);
        }
      } catch (error) {
        if (requestID === previewRequestID.current) setPreviewError(errorMessage(error));
      } finally {
        if (requestID === previewRequestID.current) setLoading(false);
      }
    },
    [agentName, onPreview],
  );

  const { selectTarget, selectDefault } = useRuntimeTargetSelectors(
    loadPreview,
    setActiveJobID,
    setSelectedTarget,
    setSelectedUseDefault,
  );

  useEffect(() => {
    if (open) void loadPreview();
  }, [loadPreview, open]);

  const handleOpenChange = (nextOpen: boolean) => handleDialogOpenChange(nextOpen, setOpen, reset);

  const approve = useCallback(
    () =>
      approveRuntimeUpdate({
        requestID: previewRequestID.current,
        previewRequestID,
        agentName,
        preview,
        selectedTarget,
        selectedUseDefault,
        onUpdate,
        setStarting,
        setApproveError,
        setActiveJobID,
      }),
    [agentName, onUpdate, preview, selectedTarget, selectedUseDefault],
  );

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
    selectDefault,
    selectedTarget,
    selectedUseDefault,
    starting,
  };
}
