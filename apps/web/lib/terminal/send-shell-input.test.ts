import { describe, it, expect, vi, beforeEach } from "vitest";
import { useShellModifiersStore } from "./shell-modifiers";
import { setActiveTerminalSender } from "./mobile-active-terminal";

const sendMock = vi.fn();
let clientFactory: () => { send: typeof sendMock } | null = () => ({ send: sendMock });

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => clientFactory(),
}));

// Imported after the mock so it picks up the mocked module.
const { sendShellInput, sendActiveTerminalInput } = await import("./send-shell-input");

const SESSION = "sess-1";

beforeEach(() => {
  sendMock.mockReset();
  clientFactory = () => ({ send: sendMock });
  useShellModifiersStore.getState().reset();
  setActiveTerminalSender(null);
});

function lastFrameData(): string | undefined {
  const call = sendMock.mock.calls.at(-1)?.[0] as { payload?: { data?: string } } | undefined;
  return call?.payload?.data;
}

describe("sendShellInput", () => {
  it("no-ops on empty data", () => {
    sendShellInput(SESSION, "");
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("sends raw data and shapes the WS frame", () => {
    sendShellInput(SESSION, "ls");
    expect(sendMock).toHaveBeenCalledWith({
      type: "request",
      action: "shell.input",
      payload: { session_id: SESSION, data: "ls" },
    });
  });

  it("applies the Ctrl chord transform when Ctrl is latched and consumes it", () => {
    useShellModifiersStore.getState().toggleCtrl();
    sendShellInput(SESSION, "c");
    expect(lastFrameData()).toBe("\x03");
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });
  });

  it("keeps Ctrl latched when sticky after a chord", () => {
    useShellModifiersStore.getState().toggleCtrl();
    useShellModifiersStore.getState().toggleCtrl();
    expect(useShellModifiersStore.getState().ctrl.sticky).toBe(true);

    sendShellInput(SESSION, "a");
    expect(lastFrameData()).toBe("\x01");
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: true });
  });

  it("translates Shift+Tab to CSI Z and consumes Shift", () => {
    useShellModifiersStore.getState().toggleShift();
    sendShellInput(SESSION, "\t");
    expect(lastFrameData()).toBe("\x1b[Z");
    expect(useShellModifiersStore.getState().shift).toEqual({ latched: false, sticky: false });
  });

  it("Shift on a non-Tab character is a passthrough; modifier still consumes", () => {
    useShellModifiersStore.getState().toggleShift();
    sendShellInput(SESSION, "a");
    expect(lastFrameData()).toBe("a");
    expect(useShellModifiersStore.getState().shift).toEqual({ latched: false, sticky: false });
  });

  it("when the WS client is null: nothing sent AND modifiers stay armed", () => {
    clientFactory = () => null;
    useShellModifiersStore.getState().toggleCtrl();
    sendShellInput(SESSION, "c");
    expect(sendMock).not.toHaveBeenCalled();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: false });
  });

  describe("with an active mobile terminal sender registered", () => {
    it("routes input through the active sender and bypasses WS", () => {
      const sender = vi.fn();
      setActiveTerminalSender(sender);
      sendShellInput(SESSION, "ls");
      expect(sender).toHaveBeenCalledWith("ls");
      expect(sendMock).not.toHaveBeenCalled();
    });

    it("applies modifiers and consumes them after a successful sender call", () => {
      const sender = vi.fn();
      setActiveTerminalSender(sender);
      useShellModifiersStore.getState().toggleCtrl();
      sendShellInput(SESSION, "c");
      expect(sender).toHaveBeenCalledWith("\x03");
      expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });
    });

    it("falls back to WS when the sender throws", () => {
      const sender = vi.fn(() => {
        throw new Error("xterm gone");
      });
      setActiveTerminalSender(sender);
      sendShellInput(SESSION, "ls");
      expect(sender).toHaveBeenCalledWith("ls");
      expect(sendMock).toHaveBeenCalledWith({
        type: "request",
        action: "shell.input",
        payload: { session_id: SESSION, data: "ls" },
      });
    });
  });
});

describe("agent terminal input", () => {
  it("never falls back to shell input when the agent socket is absent", () => {
    useShellModifiersStore.getState().toggleCtrl();
    expect(sendActiveTerminalInput(SESSION, "c")).toBe(false);
    expect(sendMock).not.toHaveBeenCalled();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
  });
  it("preserves modifiers when a disconnected sender refuses input", () => {
    const sender = vi.fn(() => false);
    setActiveTerminalSender(sender, SESSION);
    useShellModifiersStore.getState().toggleCtrl();
    expect(sendActiveTerminalInput(SESSION, "c")).toBe(false);
    expect(sender).toHaveBeenCalledWith("\x03");
    expect(sendMock).not.toHaveBeenCalled();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
  });
  it("applies modifiers once and keeps input in the active agent session", () => {
    const sender = vi.fn(() => true);
    setActiveTerminalSender(sender, SESSION);
    useShellModifiersStore.getState().toggleCtrl();
    expect(sendActiveTerminalInput("another-session", "c")).toBe(false);
    expect(sender).not.toHaveBeenCalled();
    expect(sendActiveTerminalInput(SESSION, "c")).toBe(true);
    expect(sender).toHaveBeenCalledTimes(1);
    expect(sender).toHaveBeenCalledWith("\x03");
    expect(sendMock).not.toHaveBeenCalled();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(false);
  });
});
