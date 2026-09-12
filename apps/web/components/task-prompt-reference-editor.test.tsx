import { createRef } from "react";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
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

afterEach(cleanup);

function renderEditor(value: string, ref = createRef<RichTextInputHandle>()) {
  return {
    ref,
    ...render(
      <StateProvider initialState={{ prompts: { items: [PROMPT], loaded: true, loading: false } }}>
        <TaskPromptReferenceEditor
          ref={ref}
          value={value}
          onChange={() => undefined}
          placeholder="Describe the task"
        />
      </StateProvider>,
    ),
  };
}

describe("TaskPromptReferenceEditor", () => {
  it("renders recognized aliases as editable reference chips and preserves plain text", async () => {
    const { ref } = renderEditor(`Before @${PROMPT.name}`);

    expect(
      screen.getByTestId("task-description-input").getAttribute("data-prompt-reference-editor"),
    ).toBe("true");
    await waitFor(() =>
      expect(screen.getByTestId("custom-prompt-mention").textContent).toBe(`@${PROMPT.name}`),
    );
    expect(screen.getByTestId("task-prompt-reference-remove")).toBeTruthy();
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
      expect(screen.getAllByTestId("task-prompt-reference-remove")).toHaveLength(2),
    );
    act(() => screen.getAllByTestId("task-prompt-reference-remove")[0].click());

    await waitFor(() => {
      expect(ref.current?.getValue()).toBe(` and @${PROMPT.name}`);
      expect(onChange).toHaveBeenCalledWith(` and @${PROMPT.name}`);
    });
  });
});
