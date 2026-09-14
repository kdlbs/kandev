"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { ExportReview } from "@/lib/api/domains/canvas-distribution-api";

// i18n-exempt: registry discriminator in the copyable example, not user-facing prose.
const REGISTRY_CANVAS_KIND = "canvas";

export function CanvasShareHelp({ review }: { review: ExportReview | null }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const example = useMemo(
    () =>
      [
        `- id: ${review?.metadata.package_id ?? t("canvases:examplePackageId")}`,
        `  repo: ${t("canvases:exampleRepository")}`,
        `  kind: ${REGISTRY_CANVAS_KIND}`,
        `  previews:`,
        `    - url: ${t("canvases:examplePreviewUrl")}`,
        `      alt: ${t("canvases:examplePreviewAlt")}`,
      ].join("\n"),
    [review?.metadata.package_id, t],
  );

  const copy = async () => {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(example);
    } else {
      const textarea = document.createElement("textarea");
      textarea.value = example;
      textarea.setAttribute("readonly", "true");
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      textarea.remove();
    }
    setCopied(true);
  };

  return (
    <details className="rounded-lg border border-border/70 p-4">
      <summary className="cursor-pointer font-medium">{t("canvases:howToShare")}</summary>
      <div className="mt-3 space-y-3 text-sm text-muted-foreground">
        <p>{t("canvases:shareStepDownload")}</p>
        <p>{t("canvases:shareStepRepository")}</p>
        <p>{t("canvases:shareStepRelease")}</p>
        <p>{t("canvases:shareStepSend")}</p>
        <div className="space-y-2">
          <p>{t("canvases:registryExample")}</p>
          <pre className="overflow-x-auto rounded-md bg-muted p-3 text-xs text-foreground">
            {example}
          </pre>
          <Button
            type="button"
            variant="outline"
            className="min-h-11 cursor-pointer"
            onClick={() => void copy()}
          >
            {copied ? t("canvases:registryExampleCopied") : t("canvases:copyRegistryExample")}
          </Button>
        </div>
      </div>
    </details>
  );
}
