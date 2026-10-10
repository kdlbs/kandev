import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSessionModel } from "./use-session-model";

const mocks = vi.hoisted(() => ({
  state: {
    settingsAgents: { items: [] },
    sessionModels: {
      bySessionId: {} as Record<string, { currentModelId: string; settingsPolicy?: string }>,
    },
    activeModel: { bySessionId: {} as Record<string, string> },
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

describe("useSessionModel", () => {
  beforeEach(() => {
    mocks.state.sessionModels.bySessionId = {};
    mocks.state.activeModel.bySessionId = {};
  });

  it("does not carry a stale active model into the first prompt after provider restoration", () => {
    mocks.state.sessionModels.bySessionId.session = {
      currentModelId: "provider-effective-model",
      settingsPolicy: "provider_restored",
    };
    mocks.state.activeModel.bySessionId.session = "saved-request-model";

    const { result } = renderHook(() => useSessionModel("session", undefined));

    expect(result.current).toEqual({
      sessionModel: "provider-effective-model",
      activeModel: null,
    });
  });

  it("preserves ordinary active model selection behavior", () => {
    mocks.state.sessionModels.bySessionId.session = { currentModelId: "provider-model" };
    mocks.state.activeModel.bySessionId.session = "user-selected-model";

    const { result } = renderHook(() => useSessionModel("session", undefined));

    expect(result.current).toEqual({
      sessionModel: "provider-model",
      activeModel: "user-selected-model",
    });
  });
});
