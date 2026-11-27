import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../client", () => ({
  fetchJson: vi.fn(() => Promise.resolve({ turns: [], session_id: "s" })),
}));

import { fetchJson } from "../client";
import {
  getSessionUsageTotals,
  getSessionUsageTurn,
  listSessionUsageTurns,
} from "./conversation-usage-api";

afterEach(() => vi.clearAllMocks());

describe("conversation usage API", () => {
  it("encodes task, session, and turn identities in authorized routes", async () => {
    await getSessionUsageTotals("task/a", "session a");
    await listSessionUsageTurns("task/a", "session a");
    await getSessionUsageTurn("task/a", "session a", "turn/a");

    expect(fetchJson).toHaveBeenNthCalledWith(
      1,
      "/api/v1/tasks/task%2Fa/sessions/session%20a/usage",
      undefined,
    );
    expect(fetchJson).toHaveBeenNthCalledWith(
      2,
      "/api/v1/tasks/task%2Fa/sessions/session%20a/usage/turns?limit=1",
      undefined,
    );
    expect(fetchJson).toHaveBeenNthCalledWith(
      3,
      "/api/v1/tasks/task%2Fa/sessions/session%20a/usage/turns/turn%2Fa",
      undefined,
    );
  });
});
