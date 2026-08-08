"use client";

import { useState } from "react";
import { IconCheck, IconCopy, IconTrash, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { Annotation } from "@/lib/preview-inspect-bridge";
import { formatAnnotations } from "@/lib/preview-inspect-bridge";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

interface AnnotationsPanelProps {
  annotations: Annotation[];
  onRemove: (id: string) => void;
  onClear: () => void;
}

// Module-level `t` rather than a hook: this runs from render, after a locale is
// active. The `tag`/`#id`/`.class` suffix is inspected DOM data, never copy.
function describeAnnotation(a: Annotation): string {
  if (a.kind === "pin") {
    const el = a.element;
    if (!el) return t("task:annotationPin");
    let suffix = "";
    if (el.id) suffix = `#${el.id}`;
    else if (el.classes) suffix = `.${el.classes.split(/\s+/)[0]}`;
    return `${el.tag}${suffix}`;
  }
  const r = a.rect;
  return r
    ? t("task:annotationAreaSized", { w: Math.round(r.w), h: Math.round(r.h) })
    : t("task:annotationArea");
}

export function AnnotationsPanel({ annotations, onRemove, onClear }: AnnotationsPanelProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  if (annotations.length === 0) return null;

  async function handleCopy() {
    if (await copyToClipboard(formatAnnotations(annotations))) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } else {
      console.error("AnnotationsPanel: clipboard write failed");
    }
  }

  return (
    <div
      className="flex flex-col gap-1.5 px-3 py-2 rounded-md border bg-muted text-sm"
      data-testid="preview-annotations-panel"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">
          {t("task:annotationCount", { count: annotations.length })}
        </span>
        <div className="flex items-center gap-1">
          <Button
            size="sm"
            variant="outline"
            className="h-6 px-2 cursor-pointer"
            onClick={handleCopy}
            data-testid="preview-annotations-copy"
            aria-label={copied ? t("task:copied2") : t("task:copyAnnotations")}
            title={copied ? t("task:copied2") : t("task:copyAnnotationsToClipboard")}
          >
            {copied ? <IconCheck className="h-3 w-3" /> : <IconCopy className="h-3 w-3" />}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 px-2 cursor-pointer"
            onClick={onClear}
            data-testid="preview-annotations-clear"
            aria-label={t("task:clearAllAnnotations")}
            title={t("task:clearAllAnnotations")}
          >
            <IconTrash className="h-3 w-3" />
          </Button>
        </div>
      </div>
      <ul className="flex flex-col gap-1">
        {annotations.map((a) => (
          <li key={a.id} className="flex items-start gap-2" data-testid="preview-annotation-item">
            <span className="shrink-0 w-5 h-5 rounded-full bg-primary text-primary-foreground text-xs font-mono flex items-center justify-center">
              {a.number}
            </span>
            <div className="flex-1 min-w-0">
              <code className="text-xs font-mono">{describeAnnotation(a)}</code>
              {a.comment && (
                <p className="text-xs text-muted-foreground truncate" title={a.comment}>
                  {a.comment}
                </p>
              )}
            </div>
            <Button
              size="sm"
              variant="ghost"
              className="h-5 w-5 p-0 cursor-pointer shrink-0"
              onClick={() => onRemove(a.id)}
              aria-label={t("task:removeAnnotation", { number: a.number })}
              data-testid="preview-annotation-remove"
            >
              <IconX className="h-3 w-3" />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
