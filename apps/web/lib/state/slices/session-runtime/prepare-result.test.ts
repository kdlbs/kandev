import { describe, it, expect } from "vitest";
import { prepareResultToSessionState } from "./prepare-result";

describe("prepareResultToSessionState", () => {
  it("returns null when metadata is missing or has no prepare_result", () => {
    expect(prepareResultToSessionState("s1", null)).toBeNull();
    expect(prepareResultToSessionState("s1", undefined)).toBeNull();
    expect(prepareResultToSessionState("s1", {})).toBeNull();
    expect(prepareResultToSessionState("s1", { other: 1 })).toBeNull();
  });

  it("maps snake_case prepare_result into camelCase SessionPrepareState", () => {
    const result = prepareResultToSessionState("s1", {
      prepare_result: {
        status: "completed",
        error_message: "boom",
        duration_ms: 1234,
        steps: [
          {
            name: "clone",
            command: "git clone",
            status: "ok",
            output: "done",
            error: "err",
            warning: "warn",
            warning_detail: "detail",
            started_at: "2026-01-01T00:00:00Z",
            ended_at: "2026-01-01T00:00:05Z",
          },
        ],
      },
    });

    expect(result).toEqual({
      sessionId: "s1",
      status: "completed",
      errorMessage: "boom",
      durationMs: 1234,
      steps: [
        {
          name: "clone",
          command: "git clone",
          status: "ok",
          output: "done",
          error: "err",
          warning: "warn",
          warningDetail: "detail",
          startedAt: "2026-01-01T00:00:00Z",
          endedAt: "2026-01-01T00:00:05Z",
        },
      ],
    });
  });

  it("defaults status to completed and steps to empty when absent", () => {
    const result = prepareResultToSessionState("s1", { prepare_result: {} });
    expect(result).toEqual({
      sessionId: "s1",
      status: "completed",
      errorMessage: undefined,
      durationMs: undefined,
      steps: [],
    });
  });
});
