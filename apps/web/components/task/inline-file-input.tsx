"use client";

import { useRef, useEffect, useCallback } from "react";
import { useInlineEditFocus } from "@/hooks/use-inline-edit-focus";
import { Input } from "@kandev/ui/input";
import { FileIcon } from "@/components/ui/file-icon";
import { useTranslation } from "react-i18next";

type InlineFileInputProps = {
  depth: number;
  onSubmit: (name: string) => void;
  onCancel: () => void;
};

export function InlineFileInput({ depth, onSubmit, onCancel }: InlineFileInputProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);

  const doneRef = useRef(false);
  useEffect(() => {
    const frame = requestAnimationFrame(() => inputRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, []);

  const commit = useCallback(() => {
    if (doneRef.current) return;
    doneRef.current = true;
    const value = inputRef.current?.value.trim();
    if (value) onSubmit(value);
    else onCancel();
  }, [onSubmit, onCancel]);
  const { onBlur, cancelPendingBlur } = useInlineEditFocus({
    active: true,
    inputRef,
    onLeave: commit,
  });
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (doneRef.current) return;
      if (e.key === "Enter") {
        cancelPendingBlur();
        commit();
      } else if (e.key === "Escape") {
        cancelPendingBlur();
        doneRef.current = true;
        onCancel();
      }
    },
    [cancelPendingBlur, commit, onCancel],
  );

  return (
    <div
      className="flex items-center gap-1 px-2 py-0.5"
      style={{ paddingLeft: `${depth * 12 + 8 + 20}px` }}
    >
      <FileIcon
        fileName="file"
        filePath=""
        className="flex-shrink-0"
        style={{ width: "14px", height: "14px", opacity: 0.7 }}
      />
      <Input
        ref={inputRef}
        type="text"
        controlSize="none"
        className="h-5 text-xs px-1 py-0 border-muted-foreground/30"
        placeholder={t("task:filename")}
        onKeyDown={handleKeyDown}
        onBlur={onBlur}
      />
    </div>
  );
}
