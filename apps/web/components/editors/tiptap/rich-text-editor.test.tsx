import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./tiptap-plan-editor", () => ({
  TipTapPlanEditor: (props: {
    taskId: string;
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
  }) => (
    <textarea
      data-testid="fake-tiptap-plan-editor"
      data-task-id={props.taskId}
      placeholder={props.placeholder}
      value={props.value}
      onChange={(e) => props.onChange(e.target.value)}
    />
  ),
}));

vi.mock("./tiptap-plan-readonly", () => ({
  PlanReadOnlyMarkdown: (props: { content: string; className?: string; testId?: string }) => (
    <div data-testid={props.testId ?? "fake-plan-readonly"} className={props.className}>
      {props.content}
    </div>
  ),
}));

import { RichTextEditor, RichTextReadOnly } from "./rich-text-editor";

afterEach(() => {
  cleanup();
});

describe("RichTextEditor", () => {
  it("forwards taskId/value/placeholder to the underlying plan editor", () => {
    render(
      <RichTextEditor
        taskId="task_1"
        value="hello"
        onChange={() => {}}
        placeholder="Write a note..."
        testId="notes-editor"
      />,
    );

    const editor = screen.getByTestId("fake-tiptap-plan-editor");
    expect(editor.getAttribute("data-task-id")).toBe("task_1");
    expect((editor as HTMLTextAreaElement).value).toBe("hello");
    expect(editor.getAttribute("placeholder")).toBe("Write a note...");
    expect(screen.getByTestId("notes-editor")).not.toBeNull();
  });

  it("round-trips onChange", () => {
    const onChange = vi.fn();
    render(<RichTextEditor taskId="task_1" value="" onChange={onChange} />);

    const editor = screen.getByTestId("fake-tiptap-plan-editor") as HTMLTextAreaElement;
    fireEvent.change(editor, { target: { value: "updated" } });

    expect(onChange).toHaveBeenCalledWith("updated");
  });
});

describe("RichTextReadOnly", () => {
  it("forwards value/className/testId to the read-only markdown renderer", () => {
    render(<RichTextReadOnly value="**hi**" className="my-class" testId="notes-readonly" />);

    const rendered = screen.getByTestId("notes-readonly");
    expect(rendered.textContent).toBe("**hi**");
    expect(rendered.classList.contains("my-class")).toBe(true);
  });
});
