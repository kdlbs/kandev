import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";

describe("recovered selector state", () => {
  it("retains recovery provenance across explicit changes and clears it for strict startup", () => {
    const store = createAppStore();

    store
      .getState()
      .setSessionMode("session-1", "provider-mode", [], undefined, "provider_restored");
    expect(store.getState().sessionMode.bySessionId["session-1"]).toMatchObject({
      currentModeId: "provider-mode",
      settingsPolicy: "provider_restored",
    });

    store.getState().setSessionMode("session-1", "user-selected-mode", []);
    expect(store.getState().sessionMode.bySessionId["session-1"].settingsPolicy).toBe(
      "provider_restored",
    );

    store.getState().setSessionMode("session-1", "strict-mode", [], undefined, "strict");
    expect(store.getState().sessionMode.bySessionId["session-1"].settingsPolicy).toBeUndefined();

    store.getState().setActiveModel("session-1", "saved-model");
    store.getState().clearActiveModel("session-1");
    expect(store.getState().activeModel.bySessionId["session-1"]).toBeUndefined();
  });
});
