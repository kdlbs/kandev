import { act } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { setChatDraftAttachments } from "@/lib/local-storage";
import {
  FIRST,
  SECOND,
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
const SEND = "message.add";
// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.3 through 004.6
// The real action used to reacquire the mutable handle and erase newer editor content.
describe("composer plan implementation accepted payload", () => {
  it.each([1280, 390])("preserves newer input and persisted restoration at %spx", async (width) => {
    fixture.width(width);
    save();
    const transport = fixture.transport();
    const ack = transport.hold(SEND);
    const { runtime, view, tree } = mount();
    await flush();
    expectEditor(runtime, ORIGINAL);
    await clickImplement();
    const call = transport.request.mock.calls.find(([action]) => action === SEND)!;
    expect(call[1]).toMatchObject({
      task_id: TASK,
      session_id: FIRST,
      plan_mode: false,
      client_message_id: expect.any(String),
      content: expect.stringContaining(ORIGINAL),
    });
    edit(runtime);
    await flush();
    expectEditor(runtime, NEXT);
    const before = saved();
    expect(before.text).toBe(NEXT);
    expect(before.content).not.toBeNull();
    await settle(ack);
    expectEditor(runtime, NEXT);
    expect.soft(saved()).toEqual(before);
    view.rerender(tree({ editorKey: 1 }));
    await flush();
    expectEditor(runtime, NEXT);
    expect(
      runtime.store!.getState().taskPlans.byTaskId[TASK]?.implementation_started_session_id,
    ).toBe(FIRST);
    expect(transport.request).toHaveBeenCalledWith(
      "session.set_plan_mode",
      { session_id: FIRST, enabled: false },
      5000,
    );
  });

  it("preserves rich newer content directly in storage and on reopen", async () => {
    saveRich();
    const ack = fixture.transport().hold(SEND);
    const { runtime, view, tree } = mount();
    await flush();
    await clickImplement();
    edit(runtime, "Rich newer instruction with detail");
    await flush();
    const markdown = runtime.ref.current!.getValue();
    expect(markdown).toContain("`Rich newer instruction with detail`");
    const before = saved();
    expect(JSON.stringify(before.content)).toContain('"code"');
    await settle(ack);
    expectEditor(runtime, markdown, "Rich newer instruction with detail");
    expect.soft(saved()).toEqual(before);
    view.rerender(tree({ editorKey: 1 }));
    await flush();
    expectEditor(runtime, markdown, "Rich newer instruction with detail");
  });
});

describe("plan implementation acceptance and retry controls", () => {
  it("clears an unchanged accepted draft, including saved rich content", async () => {
    save();
    const ack = fixture.transport().hold(SEND);
    const { runtime } = mount();
    await flush();
    await clickImplement();
    await settle(ack);
    expectEditor(runtime, "");
    expect(saved()).toEqual({ text: "", content: null, attachments: [] });
  });

  it("keeps a rejected send retryable and clears the later accepted retry", async () => {
    save();
    const transport = fixture.transport();
    const ack = transport.hold(SEND);
    const { runtime } = mount();
    await flush();
    const before = saved();
    await clickImplement();
    await act(async () => {
      ack.reject(new Error("External delivery rejected"));
      await ack.promise.catch(() => {});
    });
    await flush();
    expectEditor(runtime, ORIGINAL);
    expect(saved()).toEqual(before);
    transport.release(SEND);
    await clickImplement();
    expectEditor(runtime, "");
    expect(transport.request.mock.calls.filter(([action]) => action === SEND)).toHaveLength(2);
  });

  it("preserves the whole draft when attachments change despite identical text", async () => {
    save();
    setChatDraftAttachments(FIRST, [
      {
        id: "original",
        attachmentId: "original",
        mimeType: "text/plain",
        fileName: "original.txt",
        size: 8,
        isImage: false,
        deliveryMode: "path",
      },
    ]);
    const transport = fixture.transport();
    const ack = transport.hold(SEND);
    const { runtime } = mount();
    await flush();
    await clickImplement();
    expect(
      transport.request.mock.calls.find(([action]) => action === SEND)![1].attachments,
    ).toEqual([
      {
        type: "resource",
        attachment_id: "original",
        mime_type: "text/plain",
        name: "original.txt",
        size_bytes: 8,
        delivery_mode: "path",
      },
    ]);
    act(() =>
      runtime.ref.current!.restoreStagedAttachments!([
        {
          type: "resource",
          attachment_id: "new-file",
          mime_type: "text/plain",
          name: "new.txt",
          size_bytes: 9,
          delivery_mode: "path",
        },
      ]),
    );
    await flush();
    const before = saved();
    const descriptors = runtime.ref.current!.getAttachments();
    expect(descriptors).toHaveLength(2);
    await settle(ack);
    expectEditor(runtime, ORIGINAL);
    expect.soft(saved()).toEqual(before);
    expect.soft(runtime.ref.current!.getAttachments()).toEqual(descriptors);
  });
});

describe("plan implementation initiating editor ownership", () => {
  it.each(["different", "identical", "A-B-A", "replacement", "unmount"])(
    "preserves successor drafts after %s",
    async (boundary) => {
      save();
      save(SECOND, boundary === "identical" ? ORIGINAL : NEXT);
      const ack = fixture.transport().hold(SEND);
      const { runtime, view, tree } = mount();
      await flush();
      await clickImplement();
      let current = runtime;
      if (boundary === "replacement") view.rerender(tree({ editorKey: 1 }));
      else if (boundary === "unmount") {
        view.unmount();
        current = mount().runtime;
      } else {
        act(() => runtime.store!.getState().setActiveSession(TASK, SECOND));
        await flush();
        if (boundary === "A-B-A")
          act(() => runtime.store!.getState().setActiveSession(TASK, FIRST));
      }
      await flush();
      const id = boundary === "different" || boundary === "identical" ? SECOND : FIRST;
      const expected = id === SECOND && boundary !== "identical" ? NEXT : ORIGINAL;
      const before = [saved(FIRST), saved(SECOND)];
      await settle(ack);
      expectEditor(current, expected);
      expect.soft([saved(FIRST), saved(SECOND)]).toEqual(before);
    },
  );

  it("a no-ref plan toolbar leaves a separately mounted composer intact", async () => {
    save();
    const ack = fixture.transport().hold(SEND);
    const { runtime } = mount({ header: true });
    await flush();
    const before = saved();
    await clickImplement(false, true);
    await settle(ack);
    expectEditor(runtime, ORIGINAL);
    expect(saved()).toEqual(before);
  });
});
