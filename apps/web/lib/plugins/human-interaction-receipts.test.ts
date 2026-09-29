import { afterEach, describe, expect, it, vi } from "vitest";
import { issueHumanInteractionResponseReceipt } from "./human-interaction-receipts";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));

afterEach(() => vi.unstubAllGlobals());

const INTERACTION_ID = "interaction-1";

describe("issueHumanInteractionResponseReceipt", () => {
  it("sends the exact response to the authenticated Host receipt endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "receipt-1",
          interaction_id: INTERACTION_ID,
          resource_version: "version-4",
          expires_at: "2026-09-26T10:01:00Z",
        }),
        { status: 201 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      issueHumanInteractionResponseReceipt({
        workspaceId: "workspace-1",
        interactionId: INTERACTION_ID,
        expectedResourceVersion: "version-4",
        response: {
          kind: "clarification",
          answers: [{ questionId: "question-1", selectedOptions: ["yes"], customText: "detail" }],
        },
      }),
    ).resolves.toEqual({
      id: "receipt-1",
      interactionId: INTERACTION_ID,
      resourceVersion: "version-4",
      expiresAt: "2026-09-26T10:01:00Z",
    });

    expect(fetchMock.mock.calls[0][0]).toBe(
      "http://api.test/api/plugins/host/interactions/response-receipts",
    );
    expect(JSON.parse(fetchMock.mock.calls[0][1]?.body as string)).toEqual({
      workspace_id: "workspace-1",
      interaction_id: INTERACTION_ID,
      expected_resource_version: "version-4",
      kind: "clarification",
      answers: [{ question_id: "question-1", selected_options: ["yes"], custom_text: "detail" }],
    });
  });
});
