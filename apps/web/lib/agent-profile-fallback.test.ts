import { describe, expect, it } from "vitest";

import { classifyAgentProfileFallback } from "./agent-profile-fallback";

describe("classifyAgentProfileFallback", () => {
  it("classifies a profile with no configured fallback as none", () => {
    expect(classifyAgentProfileFallback({ autoFallback: false, fallbackModel: "" })).toEqual({
      kind: "none",
    });
  });

  it("classifies exact-model selection before a saved explicit fallback", () => {
    expect(
      classifyAgentProfileFallback({
        requireExactModel: true,
        autoFallback: false,
        fallbackModel: "saved-explicit-model",
      }),
    ).toEqual({ kind: "exact" });
  });

  it("classifies exact-model selection before saved automatic fallback", () => {
    expect(
      classifyAgentProfileFallback({
        requireExactModel: true,
        autoFallback: true,
        fallbackModel: "",
      }),
    ).toEqual({ kind: "exact" });
  });

  it("classifies automatic fallback as next", () => {
    expect(classifyAgentProfileFallback({ autoFallback: true, fallbackModel: "" })).toEqual({
      kind: "next",
    });
  });

  it("preserves an explicit fallback model opaquely", () => {
    const model = "  provider/model:with spaces  ";
    expect(classifyAgentProfileFallback({ autoFallback: false, fallbackModel: model })).toEqual({
      kind: "model",
      model,
    });
  });

  it("gives automatic fallback precedence over an explicit saved model", () => {
    expect(
      classifyAgentProfileFallback({ autoFallback: true, fallbackModel: "saved-explicit-model" }),
    ).toEqual({ kind: "next" });
  });
});
