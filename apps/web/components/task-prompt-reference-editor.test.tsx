import { createRef, useEffect } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { RichTextInputHandle } from "./task/chat/rich-text-input";
import { TaskPromptReferenceEditor } from "./task-prompt-reference-editor";

const PROMPT = {
  id: "prompt-1",
  name: "Daily Summary",
  content: "Summarize the current work.",
  builtin: false,
  created_at: "2026-09-12T00:00:00Z",
  updated_at: "2026-09-12T00:00:00Z",
};
const PROMPT_REFERENCE_REMOVE_TEST_ID = "task-prompt-reference-remove";

afterEach(cleanup);

function PromptStoreCapture({ storeRef }: { storeRef: { current: StoreApi<AppState> | null } }) {
  const store = useAppStoreApi();
  useEffect(() => {
    storeRef.current = store;
  }, [store, storeRef]);
  return null;
}

function renderEditor(
  value: string,
  ref = createRef<RichTextInputHandle>(),
  prompts = [PROMPT],
  onChange: (nextValue: string) => void = () => undefined,
) {
  const storeRef: { current: StoreApi<AppState> | null } = { current: null };
  return {
    ref,
    storeRef,
    ...render(
      <StateProvider initialState={{ prompts: { items: prompts, loaded: true, loading: false } }}>
        <PromptStoreCapture storeRef={storeRef} />
        <TaskPromptReferenceEditor
          ref={ref}
          value={value}
          onChange={onChange}
          placeholder="Describe the task"
        />
      </StateProvider>,
    ),
  };
}

describe("TaskPromptReferenceEditor basic behavior", () => {
  it("renders recognized aliases as editable reference chips and preserves plain text", async () => {
    const { ref } = renderEditor(`Before @${PROMPT.name}`);

    expect(
      screen.getByTestId("task-description-input").getAttribute("data-prompt-reference-editor"),
    ).toBe("true");
    expect(screen.getByRole("textbox", { name: "Describe the task" })).toBeTruthy();
    await waitFor(() =>
      expect(screen.getByTestId("custom-prompt-mention").textContent).toBe(`@${PROMPT.name}`),
    );
    expect(screen.getByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toBeTruthy();
    expect(ref.current?.getValue()).toBe(`Before @${PROMPT.name}`);
  });

  it("removes only the selected occurrence through its visible action", async () => {
    const onChange = vi.fn();
    const ref = createRef<RichTextInputHandle>();
    render(
      <StateProvider initialState={{ prompts: { items: [PROMPT], loaded: true, loading: false } }}>
        <TaskPromptReferenceEditor
          ref={ref}
          value={`@${PROMPT.name} and @${PROMPT.name}`}
          onChange={onChange}
          placeholder="Describe the task"
        />
      </StateProvider>,
    );

    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(2),
    );
    act(() => screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)[0].click());

    await waitFor(() => {
      expect(ref.current?.getValue()).toBe(` and @${PROMPT.name}`);
      expect(onChange).toHaveBeenCalledWith(` and @${PROMPT.name}`);
    });
  });
});

describe("TaskPromptReferenceEditor reconciliation", () => {
  it("recognizes every alias when prompts load after a multi-paragraph draft", async () => {
    const draft = [`first @${PROMPT.name}`, `second @${PROMPT.name}`, `third @${PROMPT.name}`].join(
      "\n",
    );
    const { ref, storeRef } = renderEditor(draft, createRef<RichTextInputHandle>(), []);

    await waitFor(() => expect(storeRef.current).not.toBeNull());
    act(() => storeRef.current!.getState().setPrompts([PROMPT]));

    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(3),
    );
    expect(ref.current?.getValue()).toBe(draft);
  });

  it("removes every alias when a multi-paragraph prompt store is cleared", async () => {
    const draft = [`first @${PROMPT.name}`, `second @${PROMPT.name}`, `third @${PROMPT.name}`].join(
      "\n",
    );
    const { ref, storeRef } = renderEditor(draft);

    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(3),
    );
    act(() => storeRef.current!.getState().setPrompts([]));

    await waitFor(() => expect(screen.queryByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toBeNull());
    expect(ref.current?.getValue()).toBe(draft);
  });

  it("preserves a Unicode collapsed selection when aliases are reconciled", async () => {
    const draft = `你好😀 Before @${PROMPT.name} after\nsecond @${PROMPT.name}`;
    const caret = "你好😀 Befo".length;
    const { ref, storeRef } = renderEditor(draft, createRef<RichTextInputHandle>(), []);

    await waitFor(() => expect(ref.current).not.toBeNull());
    act(() => ref.current!.setSelectionRange(caret, caret));
    expect(ref.current?.getSelectionStart()).toBe(caret);
    act(() => storeRef.current!.getState().setPrompts([PROMPT]));

    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(2),
    );
    expect(ref.current?.getSelectionStart()).toBe(caret);
    expect(ref.current?.getSelectionEnd()).toBe(caret);
  });

  it("preserves a range selection in ordinary text when aliases are reconciled", async () => {
    const draft = `Before @${PROMPT.name} after\nother @${PROMPT.name}`;
    const start = 2;
    const end = 5;
    const { ref, storeRef } = renderEditor(draft, createRef<RichTextInputHandle>(), []);

    await waitFor(() => expect(ref.current).not.toBeNull());
    act(() => ref.current!.setSelectionRange(start, end));
    act(() => storeRef.current!.getState().setPrompts([PROMPT]));

    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(2),
    );
    expect(ref.current?.getSelectionStart()).toBe(start);
    expect(ref.current?.getSelectionEnd()).toBe(end);
  });

  it("keeps fenced code and link destinations plain across mount, blur, and store refresh", async () => {
    const draft = [
      "```",
      `@${PROMPT.name}`,
      "```",
      `[label @${PROMPT.name}](url "title @${PROMPT.name}")`,
      `after @${PROMPT.name}`,
    ].join("\n");
    const { ref, storeRef } = renderEditor(draft);

    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(2),
    );
    act(() => {
      ref.current!.focus();
      ref.current!.blur();
    });
    expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(2);

    act(() => storeRef.current!.getState().setPrompts([]));
    await waitFor(() => expect(screen.queryByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toBeNull());
    act(() => storeRef.current!.getState().setPrompts([PROMPT]));
    await waitFor(() =>
      expect(screen.getAllByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toHaveLength(2),
    );
    expect(ref.current?.getValue()).toBe(draft);
  });

  it("does not add prompt reconciliation to the undo history", async () => {
    const draft = `Before @${PROMPT.name}`;
    const { ref, storeRef } = renderEditor(draft, createRef<RichTextInputHandle>(), []);

    await waitFor(() => expect(ref.current).not.toBeNull());
    act(() => {
      ref.current!.setSelectionRange(draft.length, draft.length);
      ref.current!.insertText(" added", draft.length, draft.length);
    });
    expect(ref.current?.getValue()).toBe(`${draft} added`);

    act(() => storeRef.current!.getState().setPrompts([PROMPT]));
    await waitFor(() => expect(screen.getByTestId(PROMPT_REFERENCE_REMOVE_TEST_ID)).toBeTruthy());
    fireEvent.keyDown(screen.getByTestId("task-description-input"), {
      key: "z",
      ctrlKey: true,
    });

    await waitFor(() => expect(ref.current?.getValue()).toBe(draft));
  });
});

describe("TaskPromptReferenceEditor insertion", () => {
  it("inserts literal plain text, including HTML-like content, in synchronous calls", async () => {
    const ref = createRef<RichTextInputHandle>();
    const onChange = vi.fn();
    renderEditor("", ref, [PROMPT], onChange);

    await waitFor(() => expect(ref.current).not.toBeNull());
    const first = "<p>hello</p> &amp;\n  first";
    act(() => {
      ref.current!.setSelectionRange(0, 0);
      ref.current!.insertText(first, 0, 0);
      const cursor = ref.current!.getSelectionStart();
      ref.current!.insertText(" second", cursor, cursor);
    });

    await waitFor(() => expect(ref.current?.getValue()).toBe(`${first} second`));
    expect(onChange).toHaveBeenLastCalledWith(`${first} second`);
  });
});
