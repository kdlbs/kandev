"use client";

import { useState } from "react";
import { IconCheck, IconCopy, IconTrash, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { Annotation } from "@/lib/preview-inspect-bridge";
import { formatAnnotations } from "@/lib/preview-inspect-bridge";

interface AnnotationsPanelProps {
  annotations: Annotation[];
  onRemove: (id: string) => void;
  onClear: () => void;
}

function describeAnnotation(a: Annotation): string {
  if (a.kind === "pin") {
    const el = a.element;
    if (!el) return "Pin";
    let suffix = "";
    if (el.id) suffix = `#${el.id}`;
    else if (el.classes) suffix = `.${el.classes.split(/\s+/)[0]}`;
    return `${el.tag}${suffix}`;
  }
  const r = a.rect;
  return r ? `Area ${Math.round(r.w)}x${Math.round(r.h)}` : "Area";
}

export function AnnotationsPanel({ annotations, onRemove, onClear }: AnnotationsPanelProps) {
  const [copied, setCopied] = useState(false);
  if (annotations.length === 0) return null;

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(formatAnnotations(annotations));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error(`AnnotationsPanel: clipboard write failed: ${String(err)}`);
    }
  }

  return (
    <div
      className="flex flex-col gap-1.5 px-3 py-2 rounded-md border bg-muted text-sm"
      data-testid="preview-annotations-panel"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">
          {annotations.length} annotation{annotations.length === 1 ? "" : "s"}
        </span>
        <div className="flex items-center gap-1">
          <Button
            size="sm"
            variant="outline"
            className="h-6 px-2 cursor-pointer"
            onClick={handleCopy}
            data-testid="preview-annotations-copy"
            aria-label={copied ? "Copied" : "Copy annotations"}
            title={copied ? "Copied" : "Copy annotations to clipboard"}
          >
            {copied ? <IconCheck className="h-3 w-3" /> : <IconCopy className="h-3 w-3" />}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 px-2 cursor-pointer"
            onClick={onClear}
            data-testid="preview-annotations-clear"
            aria-label="Clear all annotations"
            title="Clear all annotations"
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
              aria-label={`Remove annotation ${a.number}`}
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
