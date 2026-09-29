import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReplyForm } from "./reply-form";

afterEach(cleanup);

const LABEL = "Your condition";
const SEND = "Send reply";

function setup(props: Partial<Parameters<typeof ReplyForm>[0]> = {}) {
  const onSend = vi.fn();
  const onCancel = vi.fn();
  render(
    <ReplyForm busy={false} serverError={null} onSend={onSend} onCancel={onCancel} {...props} />,
  );
  return { onSend, onCancel };
}

describe("ReplyForm", () => {
  it("focuses the textarea and disables Send reply while empty or whitespace", () => {
    setup();
    expect(document.activeElement).toBe(screen.getByLabelText(LABEL));
    expect((screen.getByRole("button", { name: SEND }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(LABEL), { target: { value: "   " } });
    expect((screen.getByRole("button", { name: SEND }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("sends the trimmed text", () => {
    const { onSend } = setup();
    fireEvent.change(screen.getByLabelText(LABEL), { target: { value: "  narrower  " } });
    fireEvent.click(screen.getByRole("button", { name: SEND }));
    expect(onSend).toHaveBeenCalledWith("narrower");
  });

  it("counts code points of the trimmed text and never truncates a paste", () => {
    setup();
    const textarea = screen.getByLabelText(LABEL) as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "😀".repeat(2000) } });
    expect(screen.getByTestId("proposal-reply-counter").textContent).toBe("2000 of 2000");
    expect((screen.getByRole("button", { name: SEND }) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.change(textarea, { target: { value: "😀".repeat(2001) } });
    expect(textarea.value.length).toBe(4002);
    expect(screen.getByTestId("proposal-reply-counter").textContent).toBe("2001 of 2000");
    expect((screen.getByRole("button", { name: SEND }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("renders a text field error inline and focuses the textarea", () => {
    setup({ serverError: { message: "text is too long", field: "text" } });
    expect(screen.getByRole("alert").textContent).toBe("text is too long");
    expect(document.activeElement).toBe(screen.getByLabelText(LABEL));
  });

  it("renders another error in a focused alert", () => {
    setup({ serverError: { message: "boom", field: null } });
    expect(document.activeElement).toBe(screen.getByRole("alert"));
  });

  it("calls onCancel", () => {
    const { onCancel } = setup();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });
});
