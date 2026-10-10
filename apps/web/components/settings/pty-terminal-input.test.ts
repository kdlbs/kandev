import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useShellModifiersStore } from "@/lib/terminal/shell-modifiers";
import { sendPtyInput } from "./pty-terminal-input";

beforeEach(() => {
  vi.stubGlobal("WebSocket", { OPEN: 1 });
  useShellModifiersStore.getState().reset();
});
afterEach(() => vi.unstubAllGlobals());

describe("sendPtyInput", () => {
  it("applies and consumes a phone Ctrl latch on the local PTY socket", () => {
    const socket = { readyState: 1, send: vi.fn() };
    useShellModifiersStore.getState().toggleCtrl();
    sendPtyInput(socket, "c", true);
    expect(new TextDecoder().decode(socket.send.mock.calls[0][0])).toBe("\x03");
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(false);
  });

  it("keeps modifiers armed while the socket is unavailable", () => {
    const socket = { readyState: 3, send: vi.fn() };
    useShellModifiersStore.getState().toggleCtrl();
    sendPtyInput(socket, "c", true);
    sendPtyInput(null, "c", true);
    expect(socket.send).not.toHaveBeenCalled();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
  });

  it("sends reverse tab and keeps sticky Shift enabled", () => {
    const socket = { readyState: 1, send: vi.fn() };
    useShellModifiersStore.getState().toggleShift();
    useShellModifiersStore.getState().toggleShift();
    sendPtyInput(socket, "\t", true);
    expect(new TextDecoder().decode(socket.send.mock.calls[0][0])).toBe("\x1b[Z");
    expect(useShellModifiersStore.getState().shift.sticky).toBe(true);
  });

  it("leaves standard PTY input and modifiers unchanged", () => {
    const socket = { readyState: 1, send: vi.fn() };
    useShellModifiersStore.getState().toggleCtrl();
    sendPtyInput(socket, "c", false);
    expect(new TextDecoder().decode(socket.send.mock.calls[0][0])).toBe("c");
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
  });
});
