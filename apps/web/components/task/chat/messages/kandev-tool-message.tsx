"use client";

import { memo } from "react";
import type { Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { extractKandevStem, extractMcpResult } from "./kandev/parse";
import { getKandevRenderer } from "./kandev/registry";
import { PermissionActionRow } from "./permission-action-row";
import { parsePermission, usePermissionResponseHandlers } from "./use-permission-handlers";

type KandevToolMessageProps = {
  comment: Message;
  permissionMessage?: Message;
};

// kandevStemOf scans the several fields that may carry the raw MCP tool name
// and returns the first one that parses to a known kandev stem. The fields
// disagree in practice:
//   - `metadata.tool_name`     — not set by the orchestrator today (null).
//   - `metadata.title`         — the raw `mcp__kandev__<tool>_kandev` string.
//   - `comment.content`        — same raw string, redundant with title.
//   - `metadata.normalized.generic.name` — the ACP adapter's *category*
//     (often `"other"`) rather than the tool name, so it cannot be matched on.
// We iterate candidates instead of picking a single "preferred" one because
// the live data showed `generic.name = "other"`, which would short-circuit
// any priority-based ordering on the wrong field.
function kandevStemOf(comment: Message): string | null {
  const meta = comment.metadata as ToolCallMetadata | undefined;
  const candidates: Array<string | undefined> = [
    meta?.tool_name,
    meta?.title,
    comment.content || undefined,
    meta?.normalized?.generic?.name,
  ];
  for (const candidate of candidates) {
    const stem = extractKandevStem(candidate);
    if (stem) return stem;
  }
  return null;
}

// hasKandevRenderer is the matcher used by the message dispatcher. It accepts
// any `tool_call` whose tool name is recognised as a Kandev MCP tool AND for
// which a per-tool renderer is registered. We require both because we still
// want unregistered Kandev tools to fall through to the generic ToolCallMessage
// (rather than rendering an empty row) until a dedicated renderer ships.
export function hasKandevRenderer(comment: Message): boolean {
  if (comment.type !== "tool_call") return false;
  return !!getKandevRenderer(kandevStemOf(comment));
}

// KandevToolMessage is the rendered entry point for every Kandev tool call.
// It parses the metadata once, looks up the per-tool renderer, and hands the
// renderer pre-parsed args + result. If the renderer lookup fails we render
// nothing rather than crashing; the matcher above guards against this in
// practice, but defensive nulling avoids a bad runtime error if the dispatcher
// rules drift out of sync with the registry.
export const KandevToolMessage = memo(function KandevToolMessage({
  comment,
  permissionMessage,
}: KandevToolMessageProps) {
  const meta = comment.metadata as ToolCallMetadata | undefined;
  const renderer = getKandevRenderer(kandevStemOf(comment));
  const { permissionMetadata, isPermissionPending } = parsePermission(permissionMessage);
  const { isResponding, handleApprove, handleReject } = usePermissionResponseHandlers({
    permissionMetadata,
    permissionMessage,
  });
  if (!renderer) return null;

  // The ACP normalizer stores MCP tool args/result inside the Generic payload:
  // `generic.input` holds the call args, `generic.output` the MCP result. We
  // also accept `metadata.args` / `metadata.result` as fallbacks so the
  // component still works for any code path that hasn't routed through the
  // normalizer yet.
  const generic = meta?.normalized?.generic;
  const argsCandidate = (generic?.input as Record<string, unknown> | undefined) ?? meta?.args;
  const args = argsCandidate && typeof argsCandidate === "object" ? argsCandidate : undefined;
  const rawResult = generic?.output ?? meta?.result;
  const result = extractMcpResult(rawResult);

  // Each renderer is a stable function-pointer pulled from the static
  // registry, so invoking it like a function (rather than via JSX) is safe
  // and avoids the lint rule against "components created during render".
  const rendered = renderer({ args, result, status: meta?.status });

  if (!isPermissionPending) return rendered;

  return (
    <>
      {rendered}
      <div className="mt-2 ml-7">
        <PermissionActionRow
          onApprove={handleApprove}
          onReject={handleReject}
          isResponding={isResponding}
        />
      </div>
    </>
  );
});
