import { describe, expect, it } from "vitest";
import * as editorState from "./dynamic-agent-profile-editor-state";

describe("dynamic profile editor save payload", () => {
  it("retains API-configured unclassified policy values", () => {
    const buildPayload = (editorState as unknown as Record<string, unknown>).dynamicProfilePayload;
    expect(typeof buildPayload).toBe("function");
    if (typeof buildPayload !== "function") return;

    const payload = buildPayload("Dynamic", true, 1, [
      {
        position: 0,
        executionProfileId: "candidate-1",
        enabled: true,
        policies: {
          version: 1,
          transient: {
            retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
            waitForReset: { enabled: false, maxWaitSeconds: 0 },
            onExhausted: "skip",
          },
          hard: {
            retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
            waitForReset: { enabled: false, maxWaitSeconds: 0 },
            onExhausted: "stop",
          },
          unclassified: { enabled: true, consecutiveFailureThreshold: 4 },
        },
      },
    ]);

    expect(payload).toMatchObject({
      dynamic: {
        candidates: [
          {
            policies: {
              unclassified: { enabled: true, consecutive_failure_threshold: 4 },
            },
          },
        ],
      },
    });
  });
});
