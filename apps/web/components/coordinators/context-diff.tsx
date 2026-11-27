"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { lineDiff, type DiffLineKind } from "@/components/task/task-plan-diff";

type ContextDiffProps = {
  before: string;
  after: string;
  "data-testid"?: string;
};

const MARKER: Record<DiffLineKind, string> = { remove: "-", add: "+", context: " " };

const LINE_CLASS: Record<DiffLineKind, string> = {
  remove: "bg-red-500/10 text-red-700 dark:text-red-300",
  add: "bg-green-500/10 text-green-700 dark:text-green-300",
  context: "text-muted-foreground",
};

/**
 * One unified column of a context's line diff. Every line starts with a text
 * marker so the change never depends on colour alone; nothing is truncated.
 */
export function ContextDiff({ before, after, ...rest }: ContextDiffProps) {
  const { t } = useTranslation();
  const lines = useMemo(() => lineDiff(before, after), [before, after]);
  return (
    <div className="space-y-1" data-testid={rest["data-testid"] ?? "context-diff"}>
      <dl className="flex flex-wrap gap-x-4 text-xs text-muted-foreground">
        <div className="flex gap-1">
          <dt className="font-mono">-</dt>
          <dd>{t("coordinator:contextDiffKeyBefore")}</dd>
        </div>
        <div className="flex gap-1">
          <dt className="font-mono">+</dt>
          <dd>{t("coordinator:contextDiffKeyAfter")}</dd>
        </div>
      </dl>
      <pre className="overflow-x-auto rounded border p-2 font-mono text-xs/relaxed">
        {lines.map((line, index) => (
          <div
            key={index}
            data-kind={line.kind}
            className={`flex whitespace-pre-wrap break-words ${LINE_CLASS[line.kind]}`}
          >
            <span aria-hidden="false" className="w-4 shrink-0 select-none">
              {MARKER[line.kind]}
            </span>
            <span className="min-w-0 break-words">{line.text}</span>
          </div>
        ))}
      </pre>
    </div>
  );
}
