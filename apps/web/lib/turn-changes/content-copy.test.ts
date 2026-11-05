import { describe, expect, it } from "vitest";
import type { TurnFileChange } from "@/lib/types/turn-changes";
import { contentUnavailableText, isSelectedContentPending } from "./content-copy";
import { ApiError } from "@/lib/api/client";

const t = (key: string) => key;

function file(
  reason?: string,
  availability: TurnFileChange["content_availability"] = "unavailable",
): TurnFileChange {
  return {
    id: "file-1",
    repository_change_id: "repo-1",
    checkout_id: "checkout-1",
    path: "src/main.ts",
    kind: "modified",
    content_availability: availability,
    content_reason: reason,
  };
}

describe("contentUnavailableText", () => {
  it.each([
    ["export_timeout", "task:turnChangesContentReasonTimeout"],
    ["export_missing_object", "task:turnChangesContentReasonMissing"],
    ["export_interrupted", "task:turnChangesContentReasonInterrupted"],
    ["size_limit", "task:turnChangesContentReasonSizeLimit"],
  ])("maps %s to its existing status copy", (reason, expected) => {
    expect(contentUnavailableText(file(reason), t)).toBe(expected);
  });

  it("falls back to the generic available status", () => {
    expect(contentUnavailableText(file(), t)).toBe("task:turnChangesContentUnavailable");
  });
});

describe("isSelectedContentPending", () => {
  const repository = (exportState: string, contentComplete = false) => ({
    id: "repo-1",
    checkout_id: "checkout-1",
    availability: "ready" as const,
    enumeration_complete: true,
    comparison_complete: true,
    content_complete: contentComplete,
    export_state: exportState as "queued" | "running" | "retrying" | "complete" | "unavailable",
  });
  const notReady = new ApiError("not ready", 404, {});

  it("keeps a genuine queued file loading while its content is pending", () => {
    expect(
      isSelectedContentPending(
        file(undefined, "pending"),
        { repositories: [repository("queued")] } as never,
        false,
        notReady,
      ),
    ).toBe(true);
  });

  it("clears a synthetic placeholder once content succeeds or the repository settles", () => {
    const synthetic = { ...file(undefined, "pending"), synthetic: true };
    expect(
      isSelectedContentPending(
        synthetic,
        { repositories: [repository("queued")] } as never,
        true,
        notReady,
      ),
    ).toBe(false);
    expect(
      isSelectedContentPending(
        synthetic,
        { repositories: [repository("complete", true)] } as never,
        false,
        notReady,
      ),
    ).toBe(false);
    expect(
      isSelectedContentPending(
        synthetic,
        { repositories: [repository("unavailable")] } as never,
        false,
        notReady,
      ),
    ).toBe(false);
  });

  it("does not mask a terminal error for a real unavailable file", () => {
    expect(
      isSelectedContentPending(
        file("unavailable"),
        { repositories: [repository("unavailable")] } as never,
        false,
        new ApiError("gone", 410, {}),
      ),
    ).toBe(false);
  });

  it("surfaces a terminal content error on a synthetic selection instead of loading forever", () => {
    const synthetic = { ...file(undefined, "pending"), synthetic: true };
    expect(
      isSelectedContentPending(
        synthetic,
        { repositories: [repository("queued")] } as never,
        false,
        new ApiError("broken", 500, {}),
      ),
    ).toBe(false);
  });
});
