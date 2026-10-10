import { describe, expect, it, vi } from "vitest";
import { registerSessionModeHandlers } from "@/lib/ws/handlers/session-mode";

function harness(taskState?: string) {
  const setSessionMode = vi.fn();
  const store = {
    getState: () => ({
      setSessionMode,
      ...(taskState ? { taskSessions: { items: { s1: { state: taskState } } } } : {}),
    }),
  } as never;
  const handlers = registerSessionModeHandlers(store);
  return { setSessionMode, handler: handlers["session.mode_changed"] };
}

describe("session.mode_changed", () => {
  // AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.2, .3
  it("carries the requested mode when the session is not in it", () => {
    const { setSessionMode, handler } = harness();

    handler!({
      payload: {
        session_id: "s1",
        current_mode_id: "default",
        requested_mode_id: "bypassPermissions",
      },
    } as never);

    expect(setSessionMode).toHaveBeenCalledWith("s1", "default", undefined, "bypassPermissions");
  });

  it("carries no requested mode when the agent applied what was asked", () => {
    const { setSessionMode, handler } = harness();

    handler!({
      payload: { session_id: "s1", current_mode_id: "bypassPermissions" },
    } as never);

    expect(setSessionMode).toHaveBeenCalledWith("s1", "bypassPermissions", undefined, undefined);
  });

  it("keeps available modes when the agent leaves a special mode", () => {
    const { setSessionMode, handler } = harness();

    handler!({
      payload: {
        session_id: "s1",
        current_mode_id: "",
        available_modes: [{ id: "default", name: "Manual" }],
      },
    } as never);

    expect(setSessionMode).toHaveBeenCalledWith(
      "s1",
      "",
      [{ id: "default", name: "Manual", description: undefined }],
      undefined,
    );
  });

  it("ignores a payload without a session", () => {
    const { setSessionMode, handler } = harness();
    handler!({ payload: { current_mode_id: "plan" } } as never);
    expect(setSessionMode).not.toHaveBeenCalled();
  });

  it("carries host restored provenance to the mode selector state", () => {
    const { setSessionMode, handler } = harness();

    handler!({
      payload: {
        session_id: "s1",
        current_mode_id: "provider-mode",
        session_settings_policy: "provider_restored",
      },
    } as never);

    expect(setSessionMode).toHaveBeenCalledWith(
      "s1",
      "provider-mode",
      undefined,
      undefined,
      "provider_restored",
    );
  });

  it("clears restored provenance when a strict projection arrives after the session is waiting", () => {
    const { setSessionMode, handler } = harness("WAITING_FOR_INPUT");

    handler!({
      payload: {
        session_id: "s1",
        current_mode_id: "strict-mode",
        session_settings_policy: "strict",
      },
    } as never);

    expect(setSessionMode).toHaveBeenCalledWith(
      "s1",
      "strict-mode",
      undefined,
      undefined,
      "strict",
    );
  });
});
