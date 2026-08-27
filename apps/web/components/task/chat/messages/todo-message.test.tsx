import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Message } from "@/lib/types/http";
import { TodoMessage } from "./todo-message";

afterEach(cleanup);

function makeTodoMessage(metadata: unknown): Message {
  return {
    id: "m1",
    session_id: "s1" as Message["session_id"],
    task_id: "t1" as Message["task_id"],
    author_type: "agent",
    content: "",
    type: "todo",
    metadata,
    created_at: "",
  } as unknown as Message;
}

describe("TodoMessage malformed metadata", () => {
  it("renders nothing instead of throwing when todos is a non-array object", () => {
    expect(() =>
      render(<TodoMessage comment={makeTodoMessage({ todos: { text: "x" } })} />),
    ).not.toThrow();
    expect(screen.queryByText(/Updated Todos/)).toBeNull();
  });

  it("renders nothing instead of throwing when todos is a primitive", () => {
    expect(() => render(<TodoMessage comment={makeTodoMessage({ todos: "x" })} />)).not.toThrow();
  });

  it("renders nothing instead of throwing when metadata is a primitive", () => {
    expect(() => render(<TodoMessage comment={makeTodoMessage("not-an-object")} />)).not.toThrow();
  });

  it("drops null and primitive entries, keeping valid ones", () => {
    render(
      <TodoMessage
        comment={makeTodoMessage({ todos: [null, 42, { text: "Valid" }, { text: "" }] })}
        defaultExpanded
      />,
    );
    expect(screen.getByText(/Updated Todos/)).toBeTruthy();
    expect(screen.getByText("Valid")).toBeTruthy();
    // The empty-text entry must not count toward the total.
    expect(screen.getByText(/\(0\/1\)/)).toBeTruthy();
    expect(screen.queryByText(/\(0\/2\)/)).toBeNull();
  });

  it("renders nothing instead of throwing when previous_todo_snapshots is not an array", () => {
    expect(() =>
      render(
        <TodoMessage
          comment={makeTodoMessage({ todos: [{ text: "A" }], previous_todo_snapshots: {} })}
          defaultExpanded
        />,
      ),
    ).not.toThrow();
    expect(screen.getByText("A")).toBeTruthy();
  });

  it("drops null and primitive snapshot entries instead of crashing on expand", () => {
    render(
      <TodoMessage
        comment={makeTodoMessage({
          todos: [{ text: "A" }],
          previous_todo_snapshots: [null, 42, { todos: [{ text: "Old" }] }],
        })}
        defaultExpanded
      />,
    );
    // The crash surface is the snapshot-history expansion.
    fireEvent.click(screen.getByRole("button", { name: /earlier update/i }));
    expect(screen.getByText(/- Old/)).toBeTruthy();
  });

  it("renders nothing instead of throwing when a snapshot's todos are malformed", () => {
    expect(() =>
      render(
        <TodoMessage
          comment={makeTodoMessage({
            todos: [{ text: "A" }],
            previous_todo_snapshots: [{ todos: "not-an-array" }, { todos: [null] }],
          })}
          defaultExpanded
        />,
      ),
    ).not.toThrow();
    fireEvent.click(screen.getByRole("button", { name: /earlier update/i }));
    expect(screen.getAllByText("A").length).toBeGreaterThanOrEqual(1);
  });
});
