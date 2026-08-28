"use client";

import { useCallback } from "react";
import { openExternalLink } from "@/lib/desktop/external-links";
import { useRouter } from "@/lib/routing/client-router";
import {
  isExternalMarkdownHref,
  resolveMarkdownFileHref,
} from "@/components/shared/markdown-components";

const INTERNAL_ROUTE_PREFIXES = [
  "/t/",
  "/tasks/",
  "/office/",
  "/settings/",
  "/github/",
  "/gitlab/",
  "/linear/",
  "/jira/",
] as const;

export type MarkdownFileLinkHandlerOptions = {
  path: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

export function isMarkdownInternalRoute(href: string): boolean {
  return (
    href === "/" ||
    href.startsWith("/?") ||
    INTERNAL_ROUTE_PREFIXES.some(
      (prefix) => href.startsWith(prefix) || href === prefix.slice(0, -1),
    )
  );
}

export function useMarkdownFileLinkHandler({
  path,
  worktreePath,
  onOpenFile,
}: MarkdownFileLinkHandlerOptions) {
  const router = useRouter();

  return useCallback(
    (href: string) => {
      const filePath = resolveMarkdownFileHref(href, worktreePath, path);
      if (filePath) {
        onOpenFile?.(filePath);
        return;
      }
      if (href.startsWith("#")) return;
      if (isMarkdownInternalRoute(href)) {
        router.push(href);
        return;
      }
      if (isExternalMarkdownHref(href)) {
        void openExternalLink(href).catch(() => undefined);
      }
    },
    [onOpenFile, path, router, worktreePath],
  );
}
