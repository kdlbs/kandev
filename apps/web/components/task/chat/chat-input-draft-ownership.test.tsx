import { StrictMode, Suspense, createRef, startTransition, useState } from "react";
import { act, cleanup, fireEvent, render, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Window as HappyDOMWindow } from "happy-dom";
import {
  getChatDraftAttachments,
  getChatDraftContent,
  getChatDraftText,
  setChatDraftAttachments,
  setChatDraftContent,
  setChatDraftText,
} from "@/lib/local-storage";
import type { ReviewComment } from "@/lib/state/slices/comments";
import {
  MESSAGE,
  FIRST,
  SECOND,
  Providers,
  Composer,
  Container,
  saveDraft,
  savedDraft,
  pendingSend,
  mountComposer,
  flushEditor,
  settle,
  expectEditor,
  type DraftState,
  type Submit,
} from "./chat-input-draft-ownership.test-helpers";

const initialWidth = window.innerWidth;
const viewport = (window as unknown as HappyDOMWindow).happyDOM;
const TEXT_MIME_TYPE = "text/plain";

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  vi.useFakeTimers();
});

afterEach(async () => {
  cleanup();
  await flushEditor();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
  viewport.setWindowSize({ width: initialWidth });
  sessionStorage.clear();
  localStorage.clear();
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.3, AC-UI-SESSION-REFRESH-EFFICIENCY-004.4
describe("accepted sends retain committed draft ownership", () => {
  it.each([
    [FIRST, SECOND],
    [SECOND, FIRST],
  ])("preserves identical drafts after switching from %s to %s", async (origin, destination) => {
    saveDraft(origin);
    saveDraft(destination);
    const send = pendingSend();
    const submit = vi.fn(() => send.promise);
    const { ref, tree, view } = mountComposer(origin, submit);
    await flushEditor();
    expectEditor(ref);
    const resetHeight = vi.fn();
    act(() => ref.current!.handleSubmit(resetHeight));
    expect(submit).toHaveBeenCalledExactlyOnceWith({ message: MESSAGE });
    view.rerender(tree(destination));
    await flushEditor();
    expectEditor(ref);
    const drafts = [savedDraft(origin), savedDraft(destination)];
    expect(drafts[1].content).not.toBeNull();
    await settle(send);
    expectEditor(ref);
    expect([savedDraft(origin), savedDraft(destination)]).toEqual(drafts);
    expect(resetHeight).not.toHaveBeenCalled();
    view.unmount();
    const restored = mountComposer(destination, () => false);
    await flushEditor();
    expectEditor(restored.ref);
  });

  it("does not revive pending acceptance after A-B-A", async () => {
    saveDraft(FIRST);
    saveDraft(SECOND);
    const send = pendingSend();
    const { ref, tree, view } = mountComposer(FIRST, () => send.promise);
    await flushEditor();
    act(() => ref.current!.handleSubmit(() => {}));
    view.rerender(tree(SECOND));
    await flushEditor();
    view.rerender(tree(FIRST));
    await flushEditor();
    const before = savedDraft(FIRST);
    await settle(send);
    expectEditor(ref);
    expect(savedDraft(FIRST)).toEqual(before);
  });
});

describe("ended composer lifecycles", () => {
  it("preserves a same-session replacement and saved content after unmount", async () => {
    saveDraft(FIRST);
    const send = pendingSend();
    const { ref, tree, view } = mountComposer(FIRST, () => send.promise);
    await flushEditor();
    act(() => ref.current!.handleSubmit(() => {}));
    const before = savedDraft(FIRST);
    view.rerender(tree(FIRST, "replacement"));
    await flushEditor();
    await settle(send);
    expectEditor(ref);
    expect(savedDraft(FIRST)).toEqual(before);
  });

  it("does not clear stored content when acceptance arrives after unmount", async () => {
    saveDraft(FIRST);
    const send = pendingSend();
    const { ref, view } = mountComposer(FIRST, () => send.promise);
    await flushEditor();
    act(() => ref.current!.handleSubmit(() => {}));
    const before = savedDraft(FIRST);
    view.unmount();
    await settle(send);
    expect(savedDraft(FIRST)).toEqual(before);
    const restored = mountComposer(FIRST, () => false);
    await flushEditor();
    expectEditor(restored.ref);
  });

  it("refuses retained submit and accepted-payload callbacks after A-B-A", async () => {
    saveDraft(FIRST);
    saveDraft(SECOND);
    const submit = vi.fn(() => false);
    const { ref, tree, view } = mountComposer(FIRST, submit);
    await flushEditor();
    const retired = ref.current!;
    view.rerender(tree(SECOND));
    await flushEditor();
    view.rerender(tree(FIRST));
    await flushEditor();
    act(() => retired.handleSubmit(() => {}));
    expect(submit).not.toHaveBeenCalled();
    let cleared = true;
    act(() => {
      cleared = retired.clearAcceptedPayload({ message: MESSAGE }, () => {});
    });
    expect(cleared).toBe(false);
    expectEditor(ref);
    // The same prerequisites still permit the current callback to submit.
    act(() => ref.current!.handleSubmit(() => {}));
    expect(submit).toHaveBeenCalledExactlyOnceWith({ message: MESSAGE });
  });
});

describe("committed visit lifecycle", () => {
  it("retires an outstanding operation across StrictMode cleanup and setup", async () => {
    saveDraft(FIRST);
    const send = pendingSend();
    const submit = vi.fn(() => send.promise);
    const ref = createRef<DraftState>();
    let commits = 0;
    const startFirstVisit = (state: DraftState) => {
      if (commits++ === 0) state.handleSubmit(() => {});
    };
    render(
      <StrictMode>
        <Providers>
          <Composer ref={ref} sessionId={FIRST} onSubmit={submit} onCommit={startFirstVisit} />
        </Providers>
      </StrictMode>,
    );
    await flushEditor();
    expect(commits).toBe(2);
    expect(submit).toHaveBeenCalledExactlyOnceWith({ message: MESSAGE });
    const before = savedDraft(FIRST);
    await settle(send);
    expectEditor(ref);
    expect(savedDraft(FIRST)).toEqual(before);
  });

  it.each(["null-session", "changed-task"])(
    "retires acceptance on %s identity changes",
    async (change) => {
      saveDraft(FIRST);
      const send = pendingSend();
      const { ref, tree, view } = mountComposer(FIRST, () => send.promise);
      await flushEditor();
      act(() => ref.current!.handleSubmit(() => {}));
      view.rerender(change === "null-session" ? tree(null) : tree(FIRST, "retained", "other-task"));
      await flushEditor();
      view.rerender(tree(FIRST));
      await flushEditor();
      const before = savedDraft(FIRST);
      await settle(send);
      expectEditor(ref);
      expect(savedDraft(FIRST)).toEqual(before);
    },
  );
});

describe("commit admission", () => {
  it("keeps the current visit admitted while a replacement render is suspended", async () => {
    saveDraft(FIRST);
    saveDraft(SECOND);
    const send = pendingSend();
    const blocked = pendingSend();
    const ref = createRef<DraftState>();
    let select!: (id: string) => void;
    function Navigation() {
      const [id, setId] = useState(FIRST);
      select = setId;
      return (
        <Composer
          ref={ref}
          sessionId={id}
          onSubmit={() => send.promise}
          suspend={id === SECOND ? blocked.promise : undefined}
        />
      );
    }
    const view = render(
      <Providers>
        <Suspense fallback={null}>
          <Navigation />
        </Suspense>
      </Providers>,
    );
    await flushEditor();
    act(() => ref.current!.handleSubmit(() => {}));
    act(() => startTransition(() => select(SECOND)));
    expectEditor(ref);
    await settle(send);
    expectEditor(ref, "");
    expect(getChatDraftText(SECOND)).toBe(MESSAGE);
    view.unmount();
    await settle(blocked, false);
  });

  it("rejects a retained accepted-payload callback while the current one can clear", async () => {
    saveDraft(FIRST);
    saveDraft(SECOND);
    const { ref, tree, view } = mountComposer(FIRST, () => false);
    await flushEditor();
    const oldClear = ref.current!.clearAcceptedPayload;
    view.rerender(tree(SECOND));
    await flushEditor();
    const before = savedDraft(SECOND);
    let cleared = true;
    act(() => {
      cleared = oldClear({ message: MESSAGE }, () => {});
    });
    expect(cleared).toBe(false);
    expectEditor(ref);
    expect(savedDraft(SECOND)).toEqual(before);
    act(() => {
      cleared = ref.current!.clearAcceptedPayload({ message: MESSAGE }, () => {});
    });
    await flushEditor();
    expect(cleared).toBe(true);
    expectEditor(ref, "");
    expect(savedDraft(SECOND)).toEqual({ text: "", content: null, attachments: [] });
  });
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
describe("current visit submission controls", () => {
  it.each([false, true])("clears current accepted input with StrictMode=%s", async (strict) => {
    saveDraft(FIRST);
    const send = pendingSend();
    const ref = createRef<DraftState>();
    const composer = (
      <Providers>
        <Composer ref={ref} sessionId={FIRST} onSubmit={() => send.promise} />
      </Providers>
    );
    render(strict ? <StrictMode>{composer}</StrictMode> : composer);
    await flushEditor();
    const resetHeight = vi.fn();
    act(() => ref.current!.handleSubmit(resetHeight));
    await settle(send);
    expectEditor(ref, "");
    expect(savedDraft(FIRST)).toEqual({ text: "", content: null, attachments: [] });
    expect(resetHeight).toHaveBeenCalledOnce();
  });

  it.each(["false", "reject"])("preserves real editor/storage on %s", async (outcome) => {
    saveDraft(FIRST);
    const send = pendingSend();
    const { ref } = mountComposer(FIRST, () => send.promise);
    await flushEditor();
    const before = savedDraft(FIRST);
    act(() => ref.current!.handleSubmit(() => {}));
    if (outcome === "false") await settle(send, false);
    else {
      const error = new Error("transport rejected");
      const logged = vi.spyOn(console, "error").mockImplementation(() => {});
      await act(async () => {
        send.reject(error);
        await send.promise.catch(() => {});
      });
      await flushEditor();
      expect(logged).toHaveBeenCalledWith("Failed to submit chat input:", error);
      logged.mockRestore();
    }
    expectEditor(ref);
    expect(savedDraft(FIRST)).toEqual(before);
  });

  it("preserves newer real editor input and clears a subsequent current send", async () => {
    saveDraft(FIRST);
    const send = pendingSend();
    const submit = vi.fn<Submit>().mockReturnValueOnce(send.promise).mockReturnValue(true);
    const { ref } = mountComposer(FIRST, submit);
    await flushEditor();
    act(() => ref.current!.handleSubmit(() => {}));
    const nextText = "Keep the next instruction";
    act(() => ref.current!.inputRef.current!.setValue(nextText));
    const before = savedDraft(FIRST);
    await settle(send);
    expectEditor(ref, nextText);
    expect(savedDraft(FIRST)).toEqual(before);
    act(() => ref.current!.handleSubmit(() => {}));
    await flushEditor();
    expect(submit).toHaveBeenLastCalledWith({ message: nextText });
    expectEditor(ref, "");
  });

  it("checks settlement even when a synchronous submitter commits a session switch", async () => {
    saveDraft(FIRST);
    saveDraft(SECOND);
    const submit = vi.fn(() => {
      switchSession();
      return true;
    });
    const { ref, tree, view } = mountComposer(FIRST, submit);
    await flushEditor();
    const switchSession = () => view.rerender(tree(SECOND));
    const before = savedDraft(SECOND);
    ref.current!.handleSubmit(() => {});
    await flushEditor();
    expect(submit).toHaveBeenCalledExactlyOnceWith({ message: MESSAGE });
    expectEditor(ref);
    expect(savedDraft(SECOND)).toEqual(before);
  });
});

function attachment(id: string) {
  return {
    id,
    attachmentId: id,
    expiresAt: "2050-01-01T00:00:00Z",
    mimeType: TEXT_MIME_TYPE,
    fileName: `${id}.txt`,
    size: 8,
    isImage: false,
    deliveryMode: "path" as const,
  };
}

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
it.each([false, true])(
  "retains existing attachment snapshot clear semantics: changed=%s",
  async (changed) => {
    saveDraft(FIRST);
    setChatDraftAttachments(FIRST, [attachment("original-file")]);
    const send = pendingSend();
    const submit = vi.fn(() => send.promise);
    const { ref } = mountComposer(FIRST, submit);
    await flushEditor();
    act(() => ref.current!.handleSubmit(() => {}));
    expect(submit).toHaveBeenCalledExactlyOnceWith({
      message: MESSAGE,
      attachments: [
        {
          type: "resource",
          attachment_id: "original-file",
          mime_type: TEXT_MIME_TYPE,
          name: "original-file.txt",
          size_bytes: 8,
          delivery_mode: "path",
        },
      ],
    });
    if (changed)
      act(() =>
        ref.current!.restoreStagedAttachments([
          {
            type: "resource",
            attachment_id: "next-file",
            mime_type: TEXT_MIME_TYPE,
            name: "next-file.txt",
            size_bytes: 9,
            delivery_mode: "path",
          },
        ]),
      );
    const descriptors = ref.current!.getAttachments();
    const stored = getChatDraftAttachments(FIRST);
    await settle(send);
    expectEditor(ref, "");
    expect(getChatDraftContent(FIRST)).toBeNull();
    expect(ref.current!.getAttachments()).toEqual(changed ? descriptors : []);
    expect(getChatDraftAttachments(FIRST)).toEqual(changed ? stored : []);
  },
);

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.4
it("preserves both sessions' ready attachments on retired acceptance", async () => {
  saveDraft(FIRST);
  saveDraft(SECOND);
  setChatDraftAttachments(FIRST, [attachment("first-file")]);
  setChatDraftAttachments(SECOND, [attachment("second-file")]);
  const send = pendingSend();
  const { ref, tree, view } = mountComposer(FIRST, () => send.promise);
  await flushEditor();
  act(() => ref.current!.handleSubmit(() => {}));
  view.rerender(tree(SECOND));
  await flushEditor();
  const before = [savedDraft(FIRST), savedDraft(SECOND)];
  const visible = ref.current!.getAttachments();
  await settle(send);
  expectEditor(ref);
  expect(ref.current!.getAttachments()).toEqual(visible);
  expect([savedDraft(FIRST), savedDraft(SECOND)]).toEqual(before);
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
it("captures review, file, task and entity metadata from the real editor", async () => {
  const reference = {
    version: 1,
    ref: "mention:v1:github:issue:demo%2Frepo:7",
    provider: "github",
    kind: "issue",
    id: "7",
    key: "demo/repo#7",
    title: "Investigate editing",
    url: "https://github.com/demo/repo/issues/7",
    scope: "demo/repo",
  };
  const message = "@notes.ts @Prior task [#demo/repo#7](https://github.com/demo/repo/issues/7)";
  setChatDraftText(FIRST, message);
  setChatDraftContent(FIRST, {
    type: "doc",
    content: [
      {
        type: "paragraph",
        content: [
          {
            type: "contextMention",
            attrs: { id: "notes.ts", kind: "file", path: "notes.ts", label: "notes.ts" },
          },
          { type: "text", text: " " },
          {
            type: "contextMention",
            attrs: {
              id: "previous-task",
              kind: "task",
              label: "Prior task",
              taskId: "previous-task",
              workflowId: "workflow",
              workflowStepId: "step",
              taskState: "WAITING_FOR_INPUT",
            },
          },
          { type: "text", text: " " },
          { type: "entityReference", attrs: reference },
        ],
      },
    ],
  });
  const comment: ReviewComment = {
    id: "comment",
    sessionId: FIRST,
    text: "Review this file",
    createdAt: "2026-10-07T00:00:00Z",
    status: "pending",
    source: "review-file",
    filePath: "notes.ts",
    repositoryName: "demo",
  };
  const send = pendingSend();
  const submit = vi.fn(() => send.promise);
  const ref = createRef<DraftState>();
  render(
    <Providers>
      <Composer
        ref={ref}
        sessionId={FIRST}
        onSubmit={submit}
        pendingCommentsByFile={{ "notes.ts": [comment] }}
      />
    </Providers>,
  );
  await flushEditor();
  expect(ref.current!.inputRef.current!.getValue()).toBe(message);
  act(() => ref.current!.handleSubmit(() => {}));
  expect(submit).toHaveBeenCalledExactlyOnceWith({
    message,
    reviewComments: [comment],
    inlineMentions: [{ path: "notes.ts", name: "notes.ts", pinned: false }],
    inlineTaskMentions: [
      {
        taskId: "previous-task",
        title: "Prior task",
        workflowId: "workflow",
        workflowStepId: "step",
        state: "WAITING_FOR_INPUT",
      },
    ],
    entityReferences: [reference],
  });
  await settle(send);
  expectEditor(ref, "");
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
it("keeps real draft input while an incomplete attachment blocks submission", async () => {
  saveDraft(FIRST);
  setChatDraftAttachments(FIRST, [
    {
      id: "incomplete",
      data: btoa("x"),
      mimeType: TEXT_MIME_TYPE,
      fileName: "missing.txt",
      size: 1,
      isImage: false,
      deliveryMode: "path",
    },
  ]);
  const submit = vi.fn(() => true);
  const { ref } = mountComposer(FIRST, submit);
  await flushEditor();
  const before = savedDraft(FIRST);
  expect(ref.current!.hasPendingAttachmentUploads).toBe(true);
  act(() => ref.current!.handleSubmit(() => {}));
  expect(submit).not.toHaveBeenCalled();
  expectEditor(ref);
  expect(savedDraft(FIRST)).toEqual(before);
  act(() => ref.current!.allItems[0].onRemove?.());
  expect(ref.current!.hasPendingAttachmentUploads).toBe(false);
  act(() => ref.current!.handleSubmit(() => {}));
  await flushEditor();
  expect(submit).toHaveBeenCalledExactlyOnceWith({ message: MESSAGE });
  expectEditor(ref, "");
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.6
it("keeps independent different-session composers and app stores isolated", async () => {
  saveDraft(FIRST);
  saveDraft(SECOND);
  const send = pendingSend();
  const first = mountComposer(FIRST, () => send.promise);
  const second = mountComposer(SECOND, () => false);
  await flushEditor();
  const before = savedDraft(SECOND);
  act(() => first.ref.current!.handleSubmit(() => {}));
  await settle(send);
  expectEditor(first.ref, "");
  expectEditor(second.ref);
  expect(savedDraft(SECOND)).toEqual(before);
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.6
it("keeps different-session composers in one app store isolated", async () => {
  saveDraft(FIRST);
  saveDraft(SECOND);
  const send = pendingSend();
  const first = createRef<DraftState>();
  const second = createRef<DraftState>();
  render(
    <Providers>
      <Composer ref={first} sessionId={FIRST} onSubmit={() => send.promise} />
      <Composer ref={second} sessionId={SECOND} onSubmit={() => false} />
    </Providers>,
  );
  await flushEditor();
  const before = savedDraft(SECOND);
  act(() => first.current!.handleSubmit(() => {}));
  await settle(send);
  expectEditor(first, "");
  expectEditor(second);
  expect(savedDraft(SECOND)).toEqual(before);
});

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.4, AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
it.each([1280, 390, 840])(
  "retains the destination through the actual container submit control at %spx",
  async (width) => {
    viewport.setWindowSize({ width });
    const matchMedia = window.matchMedia.bind(window);
    vi.spyOn(window, "matchMedia").mockImplementation((query) => {
      const media = matchMedia(query);
      if (query.includes("pointer:"))
        Object.defineProperty(media, "matches", {
          value: query.includes("coarse") === width < 1024,
          configurable: true,
        });
      return media;
    });
    saveDraft(FIRST);
    saveDraft(SECOND);
    const send = pendingSend();
    const submit = vi.fn<Submit>().mockReturnValueOnce(send.promise).mockReturnValue(true);
    const tree = (sessionId: string) => (
      <Providers>
        <Container sessionId={sessionId} onSubmit={submit} />
      </Providers>
    );
    const view = render(tree(FIRST));
    await flushEditor();
    const button = within(view.container).getByTestId("submit-message-button");
    expect(button.classList.contains("min-h-11")).toBe(width < 1024);
    fireEvent.click(button);
    expect(submit).toHaveBeenCalledExactlyOnceWith({ message: MESSAGE });
    view.rerender(tree(SECOND));
    await flushEditor();
    const before = savedDraft(SECOND);
    await settle(send);
    expect(view.container.querySelector("[contenteditable='true']")?.textContent).toBe(MESSAGE);
    expect(savedDraft(SECOND)).toEqual(before);
    view.unmount();
    const restored = render(tree(SECOND));
    await flushEditor();
    expect(restored.container.querySelector("[contenteditable='true']")?.textContent).toBe(MESSAGE);
    fireEvent.click(within(restored.container).getByTestId("submit-message-button"));
    await flushEditor();
    expect(savedDraft(SECOND)).toEqual({ text: "", content: null, attachments: [] });
  },
);
