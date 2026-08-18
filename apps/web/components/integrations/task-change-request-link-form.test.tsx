import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { TaskChangeRequestLinkForm } from "./task-change-request-link-form";

afterEach(cleanup);

const pullRequestLabel = "Pull request";
const pullRequestReference = "workspace/repository#42";

function renderForm(
  overrides: Partial<React.ComponentProps<typeof TaskChangeRequestLinkForm>> = {},
) {
  const props: React.ComponentProps<typeof TaskChangeRequestLinkForm> = {
    inputLabel: pullRequestLabel,
    placeholder: pullRequestReference,
    emptyError: "Enter a Bitbucket pull request URL or key.",
    failureMessage: "Failed to link Bitbucket pull request.",
    successMessage: "Bitbucket pull request linked",
    onSubmit: vi.fn().mockResolvedValue(undefined),
    onCancel: vi.fn(),
    onSuccess: vi.fn(),
    ...overrides,
  };
  render(
    <ToastProvider>
      <TaskChangeRequestLinkForm {...props} />
    </ToastProvider>,
  );
  return props;
}

describe("TaskChangeRequestLinkForm", () => {
  it("shows an inline provider error for an empty reference", async () => {
    const props = renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Enter a Bitbucket pull request URL or key.")).not.toBeNull();
    expect(props.onSubmit).not.toHaveBeenCalled();
    expect(props.onSuccess).not.toHaveBeenCalled();
  });

  it("submits a trimmed reference, closes, and emits the native success toast", async () => {
    const props = renderForm();
    fireEvent.change(screen.getByLabelText(pullRequestLabel), {
      target: { value: "  workspace/repository#42  " },
    });

    fireEvent.submit(screen.getByRole("button", { name: "Save" }).closest("form")!);

    await waitFor(() =>
      expect(props.onSubmit).toHaveBeenCalledWith(pullRequestReference, expect.any(AbortSignal)),
    );
    await waitFor(() => expect(props.onSuccess).toHaveBeenCalledTimes(1));
    expect(screen.getByText("Bitbucket pull request linked")).not.toBeNull();
  });

  it("keeps the form open and shows the provider failure", async () => {
    const props = renderForm({
      onSubmit: vi.fn().mockRejectedValue(new Error("Pull request was not found.")),
    });
    fireEvent.change(screen.getByLabelText(pullRequestLabel), {
      target: { value: "workspace/repository#404" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Pull request was not found.")).not.toBeNull();
    expect(props.onSuccess).not.toHaveBeenCalled();
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("disables duplicate submission but keeps cancel available while saving", async () => {
    let resolveSubmit!: () => void;
    const pending = new Promise<void>((resolve) => {
      resolveSubmit = resolve;
    });
    renderForm({ onSubmit: vi.fn().mockReturnValue(pending) });
    fireEvent.change(screen.getByLabelText(pullRequestLabel), {
      target: { value: pullRequestReference },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(
      ((await screen.findByRole("button", { name: "Saving" })) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    resolveSubmit();
  });

  it("aborts an in-flight provider mutation when cancelled", async () => {
    let submittedSignal!: AbortSignal;
    const onSubmit = vi.fn((_reference: string, signal: AbortSignal) => {
      submittedSignal = signal;
      return new Promise<void>((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    });
    const props = renderForm({ onSubmit });
    fireEvent.change(screen.getByLabelText(pullRequestLabel), {
      target: { value: pullRequestReference },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(submittedSignal.aborted).toBe(true);
    expect(props.onCancel).toHaveBeenCalledOnce();
  });
});
