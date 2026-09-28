import { describe, expect, it, vi } from "vitest";
import { registerSessionModeHandlers } from "@/lib/ws/handlers/session-mode";

function harness() {
  const setSessionMode = vi.fn();
  const store = { getState: () => ({ setSessionMode }) } as never;
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
});
