"use client";

import { IconFileOff } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { FileViewerHeader } from "./file-viewer-header";
import { useTranslation } from "react-i18next";

type FileBinaryViewerProps = {
  path: string;
  worktreePath?: string;
  headerActions?: ReactNode;
};

export function FileBinaryViewer({ path, worktreePath, headerActions }: FileBinaryViewerProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col h-full">
      <FileViewerHeader path={path} worktreePath={worktreePath} actions={headerActions} />
      <div className="flex-1 flex flex-col items-center justify-center gap-3 text-muted-foreground">
        <IconFileOff size={48} strokeWidth={1.2} />
        <div className="text-sm font-medium">{t("task:cannotPreviewThisFile")}</div>
        <div className="text-xs">{t("task:binaryFile")}</div>
      </div>
    </div>
  );
}
