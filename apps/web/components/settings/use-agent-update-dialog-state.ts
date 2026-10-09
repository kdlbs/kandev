"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentUpdateJob, AgentUpdateMode, AgentUpdatePreview } from "@/lib/api";
import { isHandledApiError } from "@/lib/api/client";
import { t } from "@/lib/i18n";

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
    targetFamily?: "v2",
  ) => Promise<AgentUpdatePreview>;
  onUpdate: (
    agentName: string,
    targetVersion: string,
    useDefault?: boolean,
    targetFamily?: "v2" | AgentUpdateMode,
    expectedRuntimeRevision?: number,
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
  setSelectedFamily,
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
  setSelectedFamily: (value: "v2" | undefined) => void;
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
  setSelectedFamily(undefined);
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

function submitRuntimeUpdate({
  agentName,
  targetVersion,
  selectedUseDefault,
  selectedFamily,
  preview,
  onUpdate,
}: {
  agentName: string;
  targetVersion: string;
  selectedUseDefault: boolean;
  selectedFamily?: "v2";
  preview: AgentUpdatePreview | null;
  onUpdate: UseAgentUpdateDialogStateOptions["onUpdate"];
}): Promise<AgentUpdateJob> {
  if (preview?.update_mode === "self_update") return onUpdate(agentName, "", false, "self_update");
  if (selectedUseDefault) return onUpdate(agentName, targetVersion, true);
  if (selectedFamily === "v2") {
    if (
      !preview ||
      preview.target_family !== "v2" ||
      typeof preview.runtime_revision !== "number"
    ) {
      throw new Error(t("agents:unableToStartUpdate"));
    }
    return onUpdate(agentName, targetVersion, false, selectedFamily, preview.runtime_revision);
  }
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
  selectedFamily,
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
  selectedFamily?: "v2";
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
    const nextJob = await submitRuntimeUpdate({
      agentName,
      targetVersion,
      selectedUseDefault,
      selectedFamily,
      preview,
      onUpdate,
    });
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

type PreviewLoader = (
  targetVersion?: string,
  useDefault?: boolean,
  targetFamily?: "v2",
) => Promise<void>;

type RuntimeSelectorOptions = {
  setTerminalJob: (value: AgentUpdateJob | null) => void;
  loadPreview: PreviewLoader;
  setActiveJobID: (value: string | null) => void;
  setSelectedTarget: (value: string) => void;
  setSelectedUseDefault: (value: boolean) => void;
  setSelectedFamily: (value: "v2" | undefined) => void;
  selectedFamily: "v2" | undefined;
  setPreview: (value: AgentUpdatePreview | null) => void;
};

type RuntimePreviewLoaderOptions = {
  agentName: string;
  onPreview: UseAgentUpdateDialogStateOptions["onPreview"];
  previewRequestID: { current: number };
  setPreview: (value: AgentUpdatePreview | null) => void;
  setPreviewError: (value: string | null) => void;
  setApproveError: (value: string | null) => void;
  setLoading: (value: boolean) => void;
  setSelectedTarget: (value: string) => void;
  setSelectedUseDefault: (value: boolean) => void;
  setSelectedFamily: (value: "v2" | undefined) => void;
};

function useRuntimePreviewLoader({
  agentName,
  onPreview,
  previewRequestID,
  setPreview,
  setPreviewError,
  setApproveError,
  setLoading,
  setSelectedTarget,
  setSelectedUseDefault,
  setSelectedFamily,
}: RuntimePreviewLoaderOptions): PreviewLoader {
  return useCallback(
    async (targetVersion?: string, useDefault = false, targetFamily?: "v2") => {
      const requestID = ++previewRequestID.current;
      setLoading(true);
      setPreviewError(null);
      setApproveError(null);
      if (targetFamily) setPreview(null);
      try {
        let nextPreview: AgentUpdatePreview;
        if (useDefault) {
          nextPreview = await onPreview(agentName, undefined, true);
        } else if (targetFamily) {
          nextPreview = await onPreview(agentName, targetVersion, false, targetFamily);
        } else {
          nextPreview = await onPreview(agentName, targetVersion);
        }
        if (requestID === previewRequestID.current) {
          setPreview(nextPreview);
          setSelectedTarget(useDefault ? DEFAULT_RUNTIME_TARGET : nextPreview.target_version);
          setSelectedUseDefault(useDefault);
          setSelectedFamily(targetFamily);
        }
      } catch (error) {
        if (requestID === previewRequestID.current) {
          setPreview(null);
          setPreviewError(errorMessage(error));
        }
      } finally {
        if (requestID === previewRequestID.current) setLoading(false);
      }
    },
    [agentName, onPreview],
  );
}

function useRuntimeSelectors({
  setTerminalJob,
  loadPreview,
  setActiveJobID,
  setSelectedTarget,
  setSelectedUseDefault,
  setSelectedFamily,
  selectedFamily,
  setPreview,
}: RuntimeSelectorOptions) {
  const selectTarget = useCallback(
    (targetVersion: string) => {
      setActiveJobID(null);
      setTerminalJob(null);
      if (selectedFamily) setPreview(null);
      setSelectedTarget(targetVersion);
      setSelectedUseDefault(false);
      setSelectedFamily(undefined);
      void loadPreview(targetVersion);
    },
    [
      loadPreview,
      selectedFamily,
      setActiveJobID,
      setTerminalJob,
      setPreview,
      setSelectedFamily,
      setSelectedTarget,
      setSelectedUseDefault,
    ],
  );
  const selectDefault = useCallback(() => {
    setActiveJobID(null);
    if (selectedFamily) setPreview(null);
    setTerminalJob(null);
    setSelectedTarget(DEFAULT_RUNTIME_TARGET);
    setSelectedUseDefault(true);
    setSelectedFamily(undefined);
    void loadPreview(undefined, true);
  }, [
    loadPreview,
    selectedFamily,
    setActiveJobID,
    setTerminalJob,
    setPreview,
    setSelectedFamily,
    setSelectedTarget,
    setSelectedUseDefault,
  ]);
  const selectMigration = useCallback(() => {
    setActiveJobID(null);
    setTerminalJob(null);
    setPreview(null);
    setSelectedTarget("");
    setSelectedUseDefault(false);
    setSelectedFamily("v2");
    void loadPreview(undefined, false, "v2");
  }, [
    loadPreview,
    setActiveJobID,
    setTerminalJob,
    setPreview,
    setSelectedFamily,
    setSelectedTarget,
    setSelectedUseDefault,
  ]);
  const selectCurrentRuntime = useCallback(() => {
    setActiveJobID(null);
    setTerminalJob(null);
    setPreview(null);
    setSelectedFamily(undefined);
    void loadPreview();
  }, [loadPreview, setActiveJobID, setTerminalJob, setPreview, setSelectedFamily]);
  return { selectTarget, selectDefault, selectMigration, selectCurrentRuntime };
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
  const [selectedFamily, setSelectedFamily] = useState<"v2">();
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
      setSelectedFamily,
      setActiveJobID,
      setTerminalJob,
    });
  }, []);

  const loadPreview = useRuntimePreviewLoader({
    agentName,
    onPreview,
    previewRequestID,
    setPreview,
    setPreviewError,
    setApproveError,
    setLoading,
    setSelectedTarget,
    setSelectedUseDefault,
    setSelectedFamily,
  });

  const { selectTarget, selectDefault, selectMigration, selectCurrentRuntime } =
    useRuntimeSelectors({
      setTerminalJob,
      loadPreview,
      setActiveJobID,
      setSelectedTarget,
      setSelectedUseDefault,
      setSelectedFamily,
      selectedFamily,
      setPreview,
    });

  useEffect(() => {
    if (open) void loadPreview();
  }, [loadPreview, open]);

  const handleOpenChange = (nextOpen: boolean) => handleDialogOpenChange(nextOpen, setOpen, reset);

  const approve = () =>
    approveRuntimeUpdate({
      requestID: previewRequestID.current,
      previewRequestID,
      agentName,
      preview,
      selectedTarget,
      selectedUseDefault,
      selectedFamily,
      onUpdate,
      setStarting,
      setApproveError,
      setActiveJobID,
      setTerminalJob,
      onHandledError: () => {
        setOpen(false);
        reset();
      },
    });

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
    selectedFamily,
    selectMigration,
    selectCurrentRuntime,
    starting,
  };
}
