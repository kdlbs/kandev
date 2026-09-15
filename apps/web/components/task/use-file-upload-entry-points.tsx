"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useFileUpload, type UploadFilesResult } from "@/hooks/use-file-upload";
import { FileUploadConflictDialog } from "./file-upload-conflict-dialog";

export type UploadPickerMode = "files" | "folder";

/**
 * Owns the two hidden file inputs, the conflict dialog, and the result toasts,
 * so both Files panels get the same upload behavior from one place.
 *
 * The picked destination is captured when the picker opens, not when it
 * resolves, because the active folder can change while the OS dialog is up.
 */
export function useFileUploadEntryPoints(sessionId: string | null) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const filesInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);
  const destinationRef = useRef<string>("");
  const pickerSessionRef = useRef<string | null>(sessionId);
  const [, setPickerOpen] = useState(false);

  const { uploads, conflicts, uploadFiles, resolveConflicts, cancelConflicts } =
    useFileUpload(sessionId);

  const report = useCallback(
    (result: UploadFilesResult) => {
      if (result.cancelled) return;
      if (result.uploaded.length > 0) {
        toast({
          title: t("task:uploadComplete", { count: result.uploaded.length }),
          description:
            result.uploaded.length === 1
              ? t("task:uploadedTo", { path: result.uploaded[0].path })
              : undefined,
        });
      }
      if (result.failed > 0) {
        toast({
          title: t("task:uploadPartiallyFailed", { count: result.failed }),
          variant: "error",
        });
      }
    },
    [t, toast],
  );

  const openPicker = useCallback(
    (mode: UploadPickerMode, destination: string) => {
      destinationRef.current = destination;
      pickerSessionRef.current = sessionId;
      const input = mode === "folder" ? folderInputRef.current : filesInputRef.current;
      if (!input) return;
      // Reset first, or picking the same file twice in a row fires no change event.
      input.value = "";
      setPickerOpen(true);
      input.click();
    },
    [sessionId],
  );

  const handlePicked = useCallback(
    async (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = event.target.files;
      setPickerOpen(false);
      if (!files || files.length === 0) return;
      if (pickerSessionRef.current !== sessionId) return;
      report(await uploadFiles(destinationRef.current, files));
    },
    [sessionId, uploadFiles, report],
  );

  const handleResolve = useCallback(
    (choices: Parameters<typeof resolveConflicts>[0]) => void resolveConflicts(choices),
    [resolveConflicts],
  );

  const elements = (
    <>
      <input
        ref={filesInputRef}
        type="file"
        multiple
        className="hidden"
        data-testid="files-upload-input"
        onChange={handlePicked}
      />
      <input
        ref={folderInputRef}
        type="file"
        multiple
        className="hidden"
        data-testid="files-upload-folder-input"
        // webkitdirectory is non-standard but universally supported in the
        // browsers and webviews Kandev targets.
        {...({ webkitdirectory: "" } as Record<string, string>)}
        onChange={handlePicked}
      />
      <FileUploadConflictDialog
        pending={conflicts}
        onResolve={handleResolve}
        onCancel={cancelConflicts}
      />
    </>
  );

  return { openPicker, elements, uploads };
}
