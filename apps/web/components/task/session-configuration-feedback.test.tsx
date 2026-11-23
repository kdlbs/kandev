import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import type { SessionConfigurationResult } from "@/lib/api/domains/session-api";
import { sessionId, taskId, type TaskSession, type Turn } from "@/lib/types/http";
import { usePickerConfigurationFeedback } from "./session-configuration-feedback";

const SESSION_A = "session-a";
const SESSION_B = "session-b";
const EXECUTION = "execution-1";
const SUBSEQUENT_EXECUTION = "execution-2";
const TIME = "2026-11-21T10:00:00.000Z";
const model = { configId: "model", label: "Model", requested: "sonnet" };
const effort = { configId: "effort", label: "Reasoning", requested: "high" };

function session(id: string, executionId: string): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId("task-1"),
    agent_execution_id: executionId,
    state: "WAITING_FOR_INPUT",
    started_at: TIME,
    updated_at: TIME,
  };
}

function startedTurn(id: string, at: string): Turn {
  return {
    id,
    session_id: sessionId(SESSION_A),
    task_id: taskId("task-1"),
    started_at: at,
    created_at: at,
    updated_at: at,
  };
}

function applied(
  value: string,
  executionId: string | undefined = EXECUTION,
): SessionConfigurationResult {
  return {
    status: "applied",
    requested: value,
    effective: value,
    confirmed: true,
    execution_id: executionId,
  };
}

function useTestPickerFeedback(owner: string) {
  const feedback = usePickerConfigurationFeedback(owner, "model");
  const store = useAppStoreApi();
  return { feedback, store };
}

function mountFeedback() {
  const view = renderHook(({ owner }) => useTestPickerFeedback(owner), {
    wrapper: StateProvider,
    initialProps: { owner: SESSION_A },
  });
  const { store } = view.result.current;
  act(() => {
    store.getState().setTaskSession(session(SESSION_A, EXECUTION));
    store.getState().setTaskSession(session(SESSION_B, EXECUTION));
  });
  return {
    ...view,
    rows: (id = SESSION_A) => store.getState().configurationFeedback.bySessionId[id] ?? [],
  };
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("picker configuration feedback", () => {
  it("keeps pre-launch feedback separate while the first turn is still missing", () => {
    const { result, rows } = mountFeedback();
    const { feedback, store } = result.current;
    act(() => {
      store.getState().setTaskSession({
        ...session(SESSION_A, EXECUTION),
        state: "CREATED",
        agent_execution_id: undefined,
      });
      feedback.showChanges(feedback.begin(), [
        {
          configId: "mode",
          label: "Agent permission mode",
          requested: "plan-mock",
          result: { status: "saved_for_launch", requested: "plan-mock", confirmed: false },
        },
      ]);
    });
    const [beforeLaunch] = rows();

    act(() => {
      store.getState().setTaskSession(session(SESSION_A, EXECUTION));
      feedback.showChanges(feedback.begin(), [{ ...model, result: applied("sonnet") }]);
    });

    expect(rows()).toHaveLength(2);
    expect(rows()[0]).toEqual(beforeLaunch);
    expect(rows()[1].entries).toEqual([
      { configId: "model", kind: "changed", label: "model", value: "sonnet" },
    ]);
  });

  it("keeps a settled result across a pause and binds late results to their initiating turn", () => {
    vi.useFakeTimers();
    const { result, rows } = mountFeedback();
    const { feedback, store } = result.current;
    act(() => {
      store.getState().addTurn(startedTurn("first", TIME));
      feedback.showChanges(feedback.begin(), [{ ...model, result: applied("sonnet") }]);
      vi.advanceTimersByTime(20_000);
    });
    expect(rows()[0].entries).toEqual([
      { configId: "model", kind: "changed", label: "model", value: "sonnet" },
    ]);

    act(() => {
      const pending = feedback.begin();
      store.getState().addTurn(startedTurn("next", "2026-11-21T10:00:30.000Z"));
      store.getState().addTurn(startedTurn("older", "2026-11-21T09:59:00.000Z"));
      feedback.showChanges(pending, [{ ...effort, result: applied("high") }], vi.fn());
      feedback.showChanges(feedback.begin(), [
        { ...model, requested: "opus", result: applied("opus") },
      ]);
    });

    expect(rows().map((row) => row.turnId)).toEqual(["first", "next"]);
    expect(rows()[0].entries).toEqual([
      { configId: "model", kind: "changed", label: "model", value: "sonnet" },
      { configId: "effort", kind: "changed", label: "reasoning", value: "high" },
    ]);
    expect(rows()[0].retry).toBeUndefined();
    expect(rows()[1].entries).toEqual([
      { configId: "model", kind: "changed", label: "model", value: "opus" },
    ]);
  });

  it("drops an obsolete request and results from a different session or execution", () => {
    const { result, rerender, rows } = mountFeedback();
    const { feedback, store } = result.current;
    act(() => {
      const old = feedback.begin();
      feedback.begin();
      feedback.showChanges(old, [{ ...model, result: applied("sonnet") }]);
    });
    expect(rows()).toEqual([]);

    let pending = 0;
    act(() => {
      pending = feedback.begin();
    });
    rerender({ owner: SESSION_B });
    act(() => feedback.showChanges(pending, [{ ...model, result: applied("sonnet") }]));
    expect(rows(SESSION_A)).toEqual([]);
    expect(rows(SESSION_B)).toEqual([]);

    act(() => {
      pending = feedback.begin();
      store.getState().setTaskSession(session(SESSION_B, SUBSEQUENT_EXECUTION));
      feedback.showChanges(pending, [
        { ...model, error: new Error("previous execution timed out") },
      ]);
    });
    expect(rows(SESSION_B)).toEqual([]);
  });
});

describe("picker configuration outcomes", () => {
  it("reports missing execution evidence and transport errors as unconfirmed", () => {
    const { result, rows } = mountFeedback();
    const { feedback } = result.current;
    act(() => {
      feedback.showChanges(feedback.begin(), [
        {
          ...model,
          result: { ...applied("sonnet"), execution_id: undefined },
        },
        {
          ...effort,
          error: new Error("request timed out"),
        },
      ]);
    });
    expect(rows()[0].entries).toEqual([
      { configId: "model", kind: "unconfirmed", label: "model", value: "sonnet", reason: "" },
      {
        configId: "effort",
        kind: "unconfirmed",
        label: "reasoning",
        value: "high",
        reason: "request timed out",
      },
    ]);
  });

  it("keeps a live-but-unsaved model and a rejected option readable in one result", () => {
    const { result, rows } = mountFeedback();
    const { feedback } = result.current;
    const notSaved = new ApiError("could not write the preference", 500, {
      configuration_result: { ...applied("sonnet"), status: "persistence_failed" },
    });
    const rejected = new ApiError("requested value is unsupported", 500, {
      configuration_result: { status: "rejected", requested: "high", confirmed: false },
    });
    act(() => {
      feedback.showChanges(feedback.begin(), [
        { ...model, error: notSaved },
        { ...effort, error: rejected },
      ]);
    });

    expect(rows()[0].entries).toEqual([
      {
        configId: "model",
        kind: "notice",
        text: "Model: sonnet applied, but it could not be saved. could not write the preference",
        dismissible: true,
      },
      {
        configId: "effort",
        kind: "notice",
        text: "Could not apply Reasoning: high. requested value is unsupported",
        dismissible: true,
      },
    ]);
  });
});
