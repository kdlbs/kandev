"use client";

import { useContext, useState, type MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import { MarkdownFileLinkContext } from "./markdown-components";
import {
  isBlockedMarkdownImageSource,
  resolveMarkdownFileTarget,
} from "@/lib/markdown/file-link-target";
import { useMarkdownImage } from "@/hooks/use-markdown-image";
import { ImagePreviewDialog } from "@/components/task/chat/image-preview-dialog";

function preventMarkdownLinkNavigation(event: MouseEvent<HTMLSpanElement>) {
  event.preventDefault();
  event.stopPropagation();
}

export function MarkdownImage({ src, alt }: { src?: string; alt?: string }) {
  const context = useContext(MarkdownFileLinkContext);
  const target = resolveMarkdownFileTarget(src, {
    workspaceRoot: context.worktreePath,
    fileRootAliases: context.fileRootAliases,
  });
  const image = useMarkdownImage(context.sessionId, target?.kind === "file" ? target.path : null);
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const { t } = useTranslation();
  if (
    target?.kind === "blocked" ||
    isBlockedMarkdownImageSource(src) ||
    image.error ||
    (image.src && failedSrc === image.src)
  ) {
    return (
      <span role="status">
        {alt} ({t("common:unavailable")})
      </span>
    );
  }
  if (!target || !context.sessionId) return <img src={src} alt={alt} className="max-w-full" />;
  if (!image.src) return <span role="status">{t("common:loading")}</span>;
  return (
    <span
      onClick={preventMarkdownLinkNavigation}
      onErrorCapture={() => setFailedSrc(image.src ?? null)}
    >
      <ImagePreviewDialog
        src={image.src}
        alt={alt ?? ""}
        thumbnailClassName="max-h-80 max-w-full object-contain"
      />
    </span>
  );
}
