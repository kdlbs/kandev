import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../client", () => ({
  fetchJson: vi.fn(() => Promise.resolve({ attempt_id: "a", state: "starting", expires_at: "" })),
}));

import { fetchJson } from "../client";
import {
  cancelAntigravityLogin,
  getAntigravityLogin,
  startAntigravityLogin,
} from "./antigravity-auth-api";

afterEach(() => vi.clearAllMocks());

describe("antigravity auth API", () => {
  it("uses the operator login lifecycle routes", async () => {
    await startAntigravityLogin();
    await getAntigravityLogin("attempt/1");
    await getAntigravityLogin();
    await cancelAntigravityLogin("attempt/1");

    expect(fetchJson).toHaveBeenNthCalledWith(1, "/api/v1/antigravity-acp/login", {
      init: { method: "POST" },
    });
    expect(fetchJson).toHaveBeenNthCalledWith(
      2,
      "/api/v1/antigravity-acp/login/attempt%2F1",
      undefined,
    );
    expect(fetchJson).toHaveBeenNthCalledWith(3, "/api/v1/antigravity-acp/login", undefined);
    expect(fetchJson).toHaveBeenNthCalledWith(4, "/api/v1/antigravity-acp/login/attempt%2F1", {
      init: { method: "DELETE" },
    });
  });
});
