import { act } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  FIRST,
  FRESH,
  ORIGINAL,
  NEXT,
  TASK,
  lifecycle,
  save,
  saveRich,
  saved,
  mount,
  flush,
  clickImplement,
  edit,
  settle,
  expectEditor,
} from "./plan-implementation-draft.test-helpers";

const fixture = lifecycle();
const LAUNCH = "session.launch";
const launched = { success: true, task_id: TASK, session_id: FRESH, state: "WAITING_FOR_INPUT" };
// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.3 through 004.6
// These controls use the production fresh-menu entry point and session launch service.
describe("fresh implementation draft settlement", () => {
  it.each([1280, 390])("preserves newer planning draft and destination at %spx", async (width) => {
    fixture.width(width);
    save();
    save(FRESH, "Destination draft");
    const transport = fixture.transport();
    const ack = transport.hold(LAUNCH);
    const { runtime, view, tree } = mount();
    await flush();
    await clickImplement(true);
    const payload = transport.request.mock.calls.find(([action]) => action === LAUNCH)![1];
    expect(payload).toMatchObject({
      task_id: TASK,
      intent: "start",
      agent_profile_id: "implementation-profile",
      executor_id: "executor",
      plan_mode: false,
      prompt: expect.stringContaining(ORIGINAL),
    });
    edit(runtime);
    await flush();
    const before = saved();
    const destination = saved(FRESH);
    expect(before.text).toBe(NEXT);
    expect(before.content).not.toBeNull();
    await settle(ack, launched);
    expect.soft(saved()).toEqual(before);
    expect.soft(saved(FRESH)).toEqual(destination);
    expectEditor(runtime, "Destination draft");
    expect(runtime.store!.getState().tasks.activeSessionId).toBe(FRESH);
    expect(
      runtime.store!.getState().taskPlans.byTaskId[TASK]?.implementation_started_session_id,
    ).toBe(FRESH);
    act(() => runtime.store!.getState().setActiveSession(TASK, FIRST));
    await flush();
    view.rerender(tree({ editorKey: 1 }));
    await flush();
    expectEditor(runtime, NEXT);
  });
});

describe("fresh implementation acceptance and retry controls", () => {
  it("clears matching accepted input while the initiating composer remains current", async () => {
    save();
    const ack = fixture.transport().hold(LAUNCH);
    const { runtime } = mount({ fixedSession: true });
    await flush();
    await clickImplement(true);
    await settle(ack, launched);
    expectEditor(runtime, "");
    expect(saved()).toEqual({ text: "", content: null, attachments: [] });
    expect(runtime.store!.getState().tasks.activeSessionId).toBe(FRESH);
  });

  it.each(["rejected", "missing-session"])(
    "preserves original draft after %s and permits retry",
    async (outcome) => {
      save();
      const transport = fixture.transport();
      const ack = transport.hold(LAUNCH);
      const { runtime } = mount({ fixedSession: true });
      await flush();
      const before = saved();
      await clickImplement(true);
      if (outcome === "rejected") {
        await act(async () => {
          ack.reject(new Error("External launch rejected"));
          await ack.promise.catch(() => {});
        });
        await flush();
      } else await settle(ack, { success: false });
      expectEditor(runtime, ORIGINAL);
      expect(saved()).toEqual(before);
      expect(runtime.store!.getState().tasks.activeSessionId).toBe(FIRST);
      transport.release(LAUNCH);
      await clickImplement(true);
      expectEditor(runtime, "");
    },
  );
});

describe("fresh implementation initiating editor ownership", () => {
  it.each(["session.set_primary", "task.plan.implementation_started"])(
    "keeps input typed while awaiting %s",
    async (boundary) => {
      saveRich();
      const transport = fixture.transport();
      const ack = transport.hold(boundary);
      const { runtime } = mount({ fixedSession: true });
      await flush();
      await clickImplement(true);
      expect(transport.request.mock.calls.some(([action]) => action === boundary)).toBe(true);
      edit(runtime, "Fresh newer rich draft");
      await flush();
      const before = saved();
      const markdown = runtime.ref.current!.getValue();
      expect(JSON.stringify(before.content)).toContain('"code"');
      await settle(
        ack,
        boundary === "task.plan.implementation_started"
          ? { id: "plan", task_id: TASK, content: "Plan" }
          : {},
      );
      expectEditor(runtime, markdown, "Fresh newer rich draft");
      expect.soft(saved()).toEqual(before);
    },
  );

  it("does not clear a same-session replacement editor", async () => {
    save();
    const ack = fixture.transport().hold(LAUNCH);
    const { runtime, view, tree } = mount({ fixedSession: true });
    await flush();
    await clickImplement(true);
    view.rerender(tree({ fixedSession: true, editorKey: 1 }));
    await flush();
    const before = saved();
    await settle(ack, launched);
    expectEditor(runtime, ORIGINAL);
    expect.soft(saved()).toEqual(before);
  });

  it("a no-ref fresh plan toolbar preserves the separate planning composer", async () => {
    save();
    const ack = fixture.transport().hold(LAUNCH);
    const { runtime } = mount({ header: true, fixedSession: true });
    await flush();
    const before = saved();
    await clickImplement(true, true);
    await settle(ack, launched);
    expectEditor(runtime, ORIGINAL);
    expect.soft(saved()).toEqual(before);
  });
});
