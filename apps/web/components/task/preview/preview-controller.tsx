"use client";

import { usePreviewPanel } from "@/hooks/use-preview-panel";

type PreviewControllerProps = {
  sessionId: string | null;
  hasDevScript?: boolean;
};

export function PreviewController({ sessionId, hasDevScript }: PreviewControllerProps) {
  usePreviewPanel({ sessionId, hasDevScript });
  return null;
}
