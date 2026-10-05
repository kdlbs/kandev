"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentUpdateJob, AgentUpdateMode, AgentUpdatePreview } from "@/lib/api";
import { isHandledApiError } from "@/lib/api/client";

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
    updateMode?: AgentUpdateMode,
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
  setTerminalJob,
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
  setTerminalJob: (value: AgentUpdateJob | null) => void;
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
  setTerminalJob(null);
}

function handleApprovalError(
  error: unknown,
  requestID: number,
  previewRequestID: { current: number },
  setApproveError: (value: string | null) => void,
  onHandledError: () => void,
) {
  if (isHandledApiError(error)) {
    onHandledError();
    return;
  }
  if (requestID === previewRequestID.current) setApproveError(errorMessage(error));
}

async function startApprovedUpdate(
  agentName: string,
  targetVersion: string,
  useDefault: boolean,
  mode: AgentUpdateMode | undefined,
  onUpdate: UseAgentUpdateDialogStateOptions["onUpdate"],
): Promise<AgentUpdateJob> {
  if (useDefault) return onUpdate(agentName, targetVersion, true);
  if (mode === "self_update") return onUpdate(agentName, "", false, "self_update");
  return onUpdate(agentName, targetVersion);
}

function captureApprovedJob(
  job: AgentUpdateJob,
  requestID: number,
  previewRequestID: { current: number },
  setActiveJobID: (value: string | null) => void,
  setTerminalJob: (value: AgentUpdateJob | null) => void,
) {
  if (requestID !== previewRequestID.current) return;
  if (!job.job_id && job.operation === "up_to_date") {
    setTerminalJob(job);
    setActiveJobID(null);
  } else {
    setActiveJobID(job.job_id);
  }
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
  setTerminalJob,
  onHandledError,
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
  onHandledError: () => void;
  setTerminalJob: (value: AgentUpdateJob | null) => void;
}) {
  const targetVersion = selectedUseDefault
    ? preview?.default_version || preview?.target_version || ""
    : selectedTarget || preview?.target_version || "";
  if (!targetVersion && preview?.update_mode !== "self_update") return;
  setStarting(true);
  setApproveError(null);
  setTerminalJob(null);
  try {
    const nextJob = await startApprovedUpdate(
      agentName,
      targetVersion,
      selectedUseDefault,
      preview?.update_mode,
      onUpdate,
    );
    captureApprovedJob(nextJob, requestID, previewRequestID, setActiveJobID, setTerminalJob);
  } catch (error) {
    handleApprovalError(error, requestID, previewRequestID, setApproveError, onHandledError);
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
  setTerminalJob: (value: AgentUpdateJob | null) => void,
  setSelectedTarget: (value: string) => void,
  setSelectedUseDefault: (value: boolean) => void,
) {
  const selectTarget = useCallback(
    (targetVersion: string) => {
      setActiveJobID(null);
      setTerminalJob(null);
      setSelectedTarget(targetVersion);
      setSelectedUseDefault(false);
      void loadPreview(targetVersion);
    },
    [loadPreview],
  );
  const selectDefault = useCallback(() => {
    setActiveJobID(null);
    setTerminalJob(null);
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
  const [terminalJob, setTerminalJob] = useState<AgentUpdateJob | null>(null);
  const previewRequestID = useRef(0);

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
      setTerminalJob,
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
    setTerminalJob,
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
        setTerminalJob,
        setActiveJobID,
        onHandledError: () => handleDialogOpenChange(false, setOpen, reset),
      }),
    [agentName, onUpdate, preview, reset, selectedTarget, selectedUseDefault],
  );

  return {
    activeJob: terminalJob ?? (activeJobID === job?.job_id ? job : undefined),
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
