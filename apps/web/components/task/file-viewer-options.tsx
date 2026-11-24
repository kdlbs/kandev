"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { usePluginRegistry } from "@/lib/plugins/registry";
import {
  canFindViewerForFileType,
  fileViewerCandidates,
} from "@/lib/plugins/file-viewer-selection";
import { getFilePreviewKind, getFileCategory } from "@/lib/utils/file-types";
import { PluginFilePreview } from "./plugin-file-preview";

/** Keeps the editor mounted so choosing a viewer preserves unsaved text. */
export function FileViewerOptions({
  path,
  repo,
  sessionId,
  children,
}: {
  path: string;
  repo?: string;
  sessionId: string | null;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  usePluginRegistry();
  const [selected, setSelected] = useState<string | null>(null);
  const identity = JSON.stringify([path, repo, sessionId]);
  const eligible = getFileCategory(path) === "text" && getFilePreviewKind(path) === "none";
  if (!eligible || !sessionId) return children;
  const hasOptions = fileViewerCandidates(path).length > 0 || canFindViewerForFileType(path);
  const viewing = selected === identity;
  return (
    <div className="flex h-full min-h-0 flex-col">
      {!viewing && hasOptions && (
        <div className="flex flex-wrap justify-end gap-2 border-b p-2">
          <Button
            variant="outline"
            className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={() => setSelected(identity)}
          >
            {t("plugins:viewerOpenWith")}
          </Button>
        </div>
      )}
      <div className={viewing ? "hidden" : "min-h-0 flex-1"}>{children}</div>
      {viewing && (
        <PluginFilePreview
          path={path}
          repo={repo}
          sessionId={sessionId}
          onOpenText={() => setSelected(null)}
        />
      )}
    </div>
  );
}
