import { describe, expect, it } from "vitest";
import { normalizeRemediationUrl } from "./remediation-url";

const WANT = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";

describe("normalizeRemediationUrl", () => {
  it("accepts the allowlisted OpenCode route", () => {
    expect(normalizeRemediationUrl(WANT)).toBe(WANT);
  });

  it.each([
    null,
    undefined,
    "",
    42,
    {},
    [],
    "not a url",
    "http://opencode.ai/workspace/wrk_123/go",
    "https://opencode.example.com/workspace/wrk_123/go",
    "https://opencode.ai/workspace/wrk_123",
    "https://opencode.ai/workspace/wrk_123/go/",
    "https://opencode.ai/workspace/wrk_123/other",
    "https://opencode.ai/workspace/wrk_123/go?source=email",
    "https://opencode.ai/workspace/wrk_123/go#fragment",
    "https://user:pass@opencode.ai/workspace/wrk_123/go",
    "https://opencode.ai:443/workspace/wrk_123/go",
    "https://opencode.ai/workspace/../go",
    "https://opencode.ai/workspace/../workspace/wrk_123/go",
    "https://opencode.ai/workspace/wrk_123/..%2Fgo",
    "https://opencode.ai/workspace/%2e%2e/workspace/wrk_123/go",
    "https://opencode.ai/workspace/wrk_123/go?",
    "https://opencode.ai/workspace/wrk_123/go#",
    "https://opencode.ai/workspace/wrk_123/go?#",
    "https://opencode.ai/workspace/wrk_123/go extra",
    "https://opencode.ai/workspace/" + "w".repeat(200) + "/go",
    `https://opencode.ai/workspace/${"w".repeat(129)}/go`,
    "https://opencode.ai/workspace/wrk_123/go.",
  ])("rejects invalid input %j", (raw) => {
    expect(normalizeRemediationUrl(raw)).toBeNull();
  });
});
