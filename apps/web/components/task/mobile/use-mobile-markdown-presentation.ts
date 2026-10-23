import { useCallback, useState } from "react";
import type { OpenFileTab } from "@/lib/types/backend";
import { isHybridMarkdownSupported } from "@/components/editors/markdown/hybrid-markdown-editor";
import { isMarkdownFileModeSupported, type MarkdownFileMode } from "../markdown-file-mode";
import { getMobileFileIdentity } from "./mobile-selected-file-state";

export function useMobileMarkdownPresentation(
  file: OpenFileTab,
  initialMode: MarkdownFileMode | undefined,
  onModeChange?: (mode: MarkdownFileMode) => void,
) {
  const identity = getMobileFileIdentity(file);
  const [lastIdentity, setLastIdentity] = useState(identity);
  const [markdownMode, setMarkdownMode] = useState<MarkdownFileMode | undefined>(initialMode);
  const [hybridMounted, setHybridMounted] = useState(initialMode === "edit");
  const [hybridFailed, setHybridFailed] = useState(false);
  const hybridAvailable = isHybridMarkdownSupported();
  const [previewMounted, setPreviewMounted] = useState(initialMode === "preview");
  if (lastIdentity !== identity) {
    setLastIdentity(identity);
    setMarkdownMode(initialMode);
    setHybridMounted(initialMode === "edit");
    setHybridFailed(false);
    setPreviewMounted(initialMode === "preview");
  }
  const handleModeChange = useCallback(
    (mode: MarkdownFileMode) => {
      if (!isMarkdownFileModeSupported(file.path, mode)) return;
      if (mode === "edit") {
        if (!hybridAvailable) return;
        setHybridFailed(false);
        setHybridMounted(true);
      }
      if (mode === "preview") setPreviewMounted(true);
      setMarkdownMode(mode);
      onModeChange?.(mode);
    },
    [file.path, hybridAvailable, onModeChange],
  );
  const handleSourceFallback = useCallback(() => {
    setHybridFailed(true);
    setHybridMounted(false);
  }, []);
  return {
    markdownMode:
      markdownMode === "edit" && (!hybridAvailable || hybridFailed)
        ? ("source" as const)
        : markdownMode,
    hybridAvailable,
    hybridFailed,
    keepHybridMounted:
      hybridMounted &&
      hybridAvailable &&
      !hybridFailed &&
      isMarkdownFileModeSupported(file.path, "edit"),
    keepPreviewMounted: previewMounted,
    handleModeChange,
    handleSourceFallback,
  };
}
