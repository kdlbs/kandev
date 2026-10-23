import { useEffect, useState } from "react";
import { isHybridMarkdownSupported } from "@/components/editors/markdown/hybrid-markdown-editor";
import { isMarkdownFileModeSupported, type MarkdownFileMode } from "./markdown-file-mode";

const MODE_ORDER: readonly MarkdownFileMode[] = ["preview", "edit", "source"];

export function useMarkdownFilePresentation({
  path,
  mode,
  onModeChange,
  onSourceFallback,
}: {
  path: string;
  mode: MarkdownFileMode;
  onModeChange: (mode: MarkdownFileMode) => void;
  onSourceFallback?: () => void;
}) {
  const hybridAvailable = isHybridMarkdownSupported();
  const supportedModes = MODE_ORDER.filter((candidate) =>
    isMarkdownFileModeSupported(path, candidate),
  );
  const [hybridFailed, setHybridFailed] = useState(false);
  const [hybridMounted, setHybridMounted] = useState(mode === "edit");
  const [previewMounted, setPreviewMounted] = useState(mode === "preview");
  const requestedMode = supportedModes.includes(mode) ? mode : "source";
  const safeMode =
    requestedMode === "edit" && (!hybridAvailable || hybridFailed) ? "source" : requestedMode;
  useEffect(() => {
    if (!isMarkdownFileModeSupported(path, mode)) onModeChange("source");
  }, [mode, onModeChange, path]);
  useEffect(() => {
    if (safeMode === "edit") setHybridMounted(true);
    if (safeMode === "preview") setPreviewMounted(true);
  }, [safeMode]);
  const handleModeChange = (nextMode: MarkdownFileMode) => {
    if (nextMode === "edit") {
      if (!hybridAvailable) return;
      setHybridFailed(false);
      setHybridMounted(true);
    }
    onModeChange(nextMode);
  };
  const handleSourceFallback = () => {
    setHybridFailed(true);
    setHybridMounted(false);
    onSourceFallback?.();
  };
  return {
    safeMode,
    supportedModes,
    hybridMounted,
    previewMounted,
    hybridAvailable,
    hybridFailed,
    handleModeChange,
    handleSourceFallback,
  };
}
