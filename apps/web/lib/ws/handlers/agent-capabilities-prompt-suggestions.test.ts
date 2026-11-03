import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { AgentCapabilitiesPayload, BackendMessageMap } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import { registerAgentCapabilitiesHandlers } from "./agent-capabilities";

function makeStore(session: TaskSession) {
  const setTaskSession = vi.fn();
  const state = {
    taskSessions: { items: { [session.id]: session } },
    setTaskSession,
    setAgentCapabilities: vi.fn(),
  } as unknown as AppState;
  return { store: { getState: () => state } as unknown as StoreApi<AppState>, setTaskSession };
}

function message(supports: boolean | undefined, rawSupport?: boolean) {
  const payload = {
    task_id: "t1",
    session_id: "s1",
    agent_id: "a1",
    supports_image: false,
    supports_audio: false,
    supports_embedded_context: false,
    supports_prompt_suggestions: supports,
    native_prompt_suggestions_supported: rawSupport,
    auth_methods: [],
    timestamp: "2026-10-07T00:00:00Z",
  } as AgentCapabilitiesPayload;
  return {
    action: "session.agent_capabilities",
    payload,
  } as BackendMessageMap["session.agent_capabilities"];
}

// @covers AC-UI-PROMPT-SUGGEST-002.5
describe("session.agent_capabilities prompt suggestion source", () => {
  it("marks the session native when the agent negotiated native suggestions", () => {
    const session = { id: "s1", metadata: { acp: {} } } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(message(true, true));
    expect(setTaskSession).toHaveBeenCalledWith({
      ...session,
      metadata: { acp: {}, prompt_suggestion_source: "native" },
    });
  });

  it("clears a native marker when a restarted agent did not negotiate it", () => {
    const session = {
      id: "s1",
      metadata: { prompt_suggestion_source: "native" },
    } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(message(false, false));
    expect(setTaskSession).toHaveBeenCalledWith({
      ...session,
      metadata: { prompt_suggestion_source: "none" },
    });
  });

  it("never marks a session native from raw provider support alone", () => {
    const session = { id: "s1", metadata: {} } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(message(false, true));
    expect(setTaskSession).not.toHaveBeenCalled();
  });

  it("clears a native marker when support survives but the session did not request it", () => {
    const session = {
      id: "s1",
      metadata: { prompt_suggestion_source: "native" },
    } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(message(false, true));
    expect(setTaskSession).toHaveBeenCalledWith({
      ...session,
      metadata: { prompt_suggestion_source: "none" },
    });
  });

  it("clears a native marker when an older producer omits effective support", () => {
    const session = {
      id: "s1",
      metadata: { prompt_suggestion_source: "native" },
    } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(
      message(undefined, true),
    );
    expect(setTaskSession).toHaveBeenCalledWith({
      ...session,
      metadata: { prompt_suggestion_source: "none" },
    });
  });

  it("retains effective behavior for an older payload without raw support", () => {
    const session = { id: "s1", metadata: {} } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(message(true));
    expect(setTaskSession).toHaveBeenCalledWith({
      ...session,
      metadata: { prompt_suggestion_source: "native" },
    });
  });

  it("leaves ordinary sessions untouched", () => {
    const session = { id: "s1", metadata: {} } as unknown as TaskSession;
    const { store, setTaskSession } = makeStore(session);
    registerAgentCapabilitiesHandlers(store)["session.agent_capabilities"]?.(message(undefined));
    expect(setTaskSession).not.toHaveBeenCalled();
  });
});
