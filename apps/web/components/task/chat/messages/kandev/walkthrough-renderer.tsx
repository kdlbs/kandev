"use client";

import { IconRoute } from "@tabler/icons-react";
import {
  IdChip,
  KandevBody,
  KandevRow,
  KeyValueRow,
  ListItemRow,
  SummaryDot,
  pluralCount,
} from "./shared";
import { pickArray, pickNumber, pickString } from "./parse";
import type { KandevRenderer } from "./types";

const DEFAULT_TITLE = "Walkthrough";
const RESULT_JSON_PREFIX = "Walkthrough saved:";

type WalkthroughStepLike = Record<string, unknown>;

function parseResultText(text: string | undefined): Record<string, unknown> | null {
  if (!text) return null;
  const trimmed = text.trim();
  const jsonStart = trimmed.startsWith(RESULT_JSON_PREFIX)
    ? trimmed.slice(RESULT_JSON_PREFIX.length).trim()
    : trimmed;
  if (!jsonStart.startsWith("{")) return null;
  try {
    const parsed = JSON.parse(jsonStart);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function resultPayload(result: unknown): unknown {
  return (
    parseResultText(typeof result === "string" ? result : undefined) ??
    parseResultText(pickString(result, "result")) ??
    parseResultText(pickString(result, "text")) ??
    result
  );
}

function titleFrom(args: unknown, result: unknown): string {
  const payload = resultPayload(result);
  return pickString(payload, "title") ?? pickString(args, "title") ?? DEFAULT_TITLE;
}

function stepsFrom(args: unknown, result: unknown): WalkthroughStepLike[] {
  const payload = resultPayload(result);
  return (
    pickArray<WalkthroughStepLike>(payload, "steps") ??
    pickArray<WalkthroughStepLike>(args, "steps") ??
    []
  );
}

function stepLocation(step: WalkthroughStepLike): string | null {
  const file = pickString(step, "file");
  const line = pickNumber(step, "line");
  if (!file || !line) return null;
  const repo = pickString(step, "repo");
  const lineEnd = pickNumber(step, "line_end");
  const lineLabel = lineEnd && lineEnd !== line ? `${line}-${lineEnd}` : `${line}`;
  return `${repo ? `${repo}:` : ""}${file}:${lineLabel}`;
}

function stepPreview(step: WalkthroughStepLike): string | null {
  const title = pickString(step, "title");
  if (title) return title;
  const text = pickString(step, "text")?.trim();
  if (!text) return null;
  return text.length > 90 ? `${text.slice(0, 90).trim()}...` : text;
}

export const ShowWalkthroughRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const title = titleFrom(args, result);
  const steps = stepsFrom(args, result);
  return (
    <KandevRow
      Icon={IconRoute}
      title={`Walkthrough: ${title}`}
      summary={
        <span className="inline-flex min-w-0 items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          <span>{pluralCount(steps.length, "step")}</span>
        </span>
      }
      status={status}
      hasExpandableContent={steps.length > 0}
    >
      <KandevBody>
        <KeyValueRow label="title">{title}</KeyValueRow>
        <div className="space-y-1.5">
          {steps.map((step, index) => {
            const location = stepLocation(step);
            const preview = stepPreview(step);
            return (
              <ListItemRow key={`${location ?? "step"}-${index}`}>
                <div className="flex min-w-0 items-baseline gap-2">
                  <span className="shrink-0 text-muted-foreground/70">Step {index + 1}</span>
                  {location && (
                    <span className="shrink-0 font-mono text-[11px] text-muted-foreground">
                      {location}
                    </span>
                  )}
                  {preview && <span className="min-w-0 truncate text-foreground">{preview}</span>}
                </div>
              </ListItemRow>
            );
          })}
        </div>
      </KandevBody>
    </KandevRow>
  );
};
