"use client";

import type { ExportFile } from "./export-types";
import { useTranslation } from "react-i18next";

interface ExportFilePreviewProps {
  file: ExportFile | null;
}

export function ExportFilePreview({ file }: ExportFilePreviewProps) {
  const { t } = useTranslation();
  if (!file) {
    return (
      <div className="flex-1 flex items-center justify-center text-sm text-muted-foreground">
        {t("office:selectAFileToPreview")}
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col min-w-0">
      <div className="px-4 py-2 border-b border-border shrink-0">
        <span className="text-sm font-mono text-muted-foreground">{file.path}</span>
      </div>
      <div className="flex-1 overflow-auto p-4">
        <pre className="text-sm font-mono whitespace-pre-wrap break-words">{file.content}</pre>
      </div>
    </div>
  );
}
