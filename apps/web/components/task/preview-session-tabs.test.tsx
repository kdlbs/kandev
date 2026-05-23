import { describe, it, expect, vi, afterEach } from "vitest";
import { render, fireEvent, screen, cleanup, waitFor } from "@testing-library/react";
import { PassthroughComposer } from "./preview-session-tabs";

const TEXTAREA_TID = "passthrough-composer-textarea";
const SUBMIT_TID = "passthrough-composer-submit";

describe("PassthroughComposer", () => {
  afterEach(() => {
    cleanup();
  });

  it("submits trimmed text on Enter and clears the textarea", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<PassthroughComposer onSubmit={onSubmit} />);

    const textarea = screen.getByTestId(TEXTAREA_TID) as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "  hello agent  " } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit).toHaveBeenCalledWith("hello agent");
    await waitFor(() => expect(textarea.value).toBe(""));
  });

  it("submits when the send button is clicked", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<PassthroughComposer onSubmit={onSubmit} />);

    fireEvent.change(screen.getByTestId(TEXTAREA_TID), {
      target: { value: "ping" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TID));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith("ping"));
  });

  it("Shift+Enter inserts a newline and does not submit", () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<PassthroughComposer onSubmit={onSubmit} />);

    const textarea = screen.getByTestId(TEXTAREA_TID);
    fireEvent.change(textarea, { target: { value: "line1" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("does not submit empty or whitespace-only input", () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<PassthroughComposer onSubmit={onSubmit} />);

    const textarea = screen.getByTestId(TEXTAREA_TID);
    fireEvent.keyDown(textarea, { key: "Enter" });
    expect(onSubmit).not.toHaveBeenCalled();

    fireEvent.change(textarea, { target: { value: "   \n   " } });
    fireEvent.keyDown(textarea, { key: "Enter" });
    expect(onSubmit).not.toHaveBeenCalled();

    const submitBtn = screen.getByTestId(SUBMIT_TID) as HTMLButtonElement;
    expect(submitBtn.disabled).toBe(true);
  });

  it("preserves the typed text when onSubmit rejects so the user can retry", async () => {
    const onSubmit = vi.fn().mockRejectedValue(new Error("network down"));
    render(<PassthroughComposer onSubmit={onSubmit} />);

    const textarea = screen.getByTestId(TEXTAREA_TID) as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "important request" } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    // Composer must NOT clear on failure — losing typed text on a flaky WS
    // disconnect was a real UX regression (issue #989 review feedback).
    expect(textarea.value).toBe("important request");
    // Composer must re-enable so the user can resubmit.
    await waitFor(() => expect(textarea.disabled).toBe(false));
  });

  it("disables the composer while a submission is in flight", async () => {
    let resolveSubmit!: () => void;
    const submitPromise = new Promise<void>((resolve) => {
      resolveSubmit = resolve;
    });
    const onSubmit = vi.fn().mockReturnValue(submitPromise);

    render(<PassthroughComposer onSubmit={onSubmit} />);
    const textarea = screen.getByTestId(TEXTAREA_TID) as HTMLTextAreaElement;
    const submitBtn = screen.getByTestId(SUBMIT_TID) as HTMLButtonElement;
    fireEvent.change(textarea, { target: { value: "hi" } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(textarea.disabled).toBe(true));
    expect(submitBtn.disabled).toBe(true);

    resolveSubmit();
    await waitFor(() => expect(textarea.disabled).toBe(false));
    expect(textarea.value).toBe("");
  });
});
