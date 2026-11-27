import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RejectForm } from "./reject-form";

afterEach(cleanup);

const REASON_LABEL = "Reason (optional)";

describe("RejectForm", () => {
  it("focuses the reason field on mount", () => {
    render(<RejectForm busy={false} serverError={null} onConfirm={vi.fn()} onCancel={vi.fn()} />);
    expect(document.activeElement).toBe(screen.getByLabelText(REASON_LABEL));
  });

  it("confirms with the trimmed reason", () => {
    const onConfirm = vi.fn();
    render(<RejectForm busy={false} serverError={null} onConfirm={onConfirm} onCancel={vi.fn()} />);
    fireEvent.change(screen.getByLabelText(REASON_LABEL), {
      target: { value: "  Not now  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Confirm reject" }));
    expect(onConfirm).toHaveBeenCalledWith("Not now");
  });

  it("confirms with undefined when the reason is left blank", () => {
    const onConfirm = vi.fn();
    render(<RejectForm busy={false} serverError={null} onConfirm={onConfirm} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Confirm reject" }));
    expect(onConfirm).toHaveBeenCalledWith(undefined);
  });

  it("calls onCancel from the Cancel button", () => {
    const onCancel = vi.fn();
    render(<RejectForm busy={false} serverError={null} onConfirm={vi.fn()} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("shows a field error under the reason field and focuses it", () => {
    render(
      <RejectForm
        busy={false}
        serverError={{ message: "Reason is too long", field: "reason" }}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert").textContent).toBe("Reason is too long");
    expect(document.activeElement).toBe(screen.getByLabelText(REASON_LABEL));
  });

  it("shows a general alert and focuses it when the error names no field", () => {
    render(
      <RejectForm
        busy={false}
        serverError={{ message: "Could not reach Kandev.", field: null }}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe("Could not reach Kandev.");
    expect(document.activeElement).toBe(alert);
  });

  it("disables both buttons and the field while busy", () => {
    render(<RejectForm busy={true} serverError={null} onConfirm={vi.fn()} onCancel={vi.fn()} />);
    expect((screen.getByLabelText(REASON_LABEL) as HTMLTextAreaElement).disabled).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Confirm reject" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("does not confirm on submit while busy", () => {
    const onConfirm = vi.fn();
    const { container } = render(
      <RejectForm busy={true} serverError={null} onConfirm={onConfirm} onCancel={vi.fn()} />,
    );
    fireEvent.submit(container.querySelector("form") as HTMLFormElement);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
