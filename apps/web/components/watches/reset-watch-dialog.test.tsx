import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ResetWatchDialog } from "./reset-watch-dialog";

afterEach(cleanup);

describe("ResetWatchDialog", () => {
  it("blocks reset in strict mode after preview failure and lets the user retry", async () => {
    const preview = vi
      .fn()
      .mockRejectedValueOnce(new Error("preview unavailable"))
      .mockResolvedValueOnce({ taskCount: 3 });
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="GitLab watch"
        requirePreviewSuccess
        previewLoader={preview}
        onConfirm={vi.fn()}
      />,
    );

    expect(await screen.findByText(/could not load the affected task count/i)).toBeTruthy();
    expect((screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry preview" }));
    await waitFor(() => expect(screen.getByText(/delete 3 tasks/i)).toBeTruthy());
    expect((screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("keeps the legacy fallback flow when strict preview gating is omitted", async () => {
    const confirm = vi.fn().mockResolvedValue(undefined);
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="GitHub watch"
        previewLoader={vi.fn().mockRejectedValue(new Error("preview unavailable"))}
        onConfirm={confirm}
      />,
    );

    expect(await screen.findByText(/delete every task previously created/i)).toBeTruthy();
    const reset = screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement;
    expect(reset.disabled).toBe(false);
    expect(screen.queryByRole("button", { name: "Retry preview" })).toBeNull();
    fireEvent.click(reset);
    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
  });

  it("keeps the existing successful preview and confirm flow", async () => {
    const confirm = vi.fn().mockResolvedValue(undefined);
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="GitHub watch"
        previewLoader={vi.fn().mockResolvedValue({ taskCount: 1 })}
        onConfirm={confirm}
      />,
    );
    expect(await screen.findByText(/delete 1 task/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
  });
});

// The description branches all changed shape when they moved into the catalog
// (whole sentences per branch, i18next plurals instead of a hand-built one).
describe("localized description branches", () => {
  const renderDialog = (props: Partial<Parameters<typeof ResetWatchDialog>[0]> = {}) =>
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="review watch"
        previewLoader={vi.fn().mockResolvedValue({ taskCount: 0 })}
        onConfirm={vi.fn()}
        {...props}
      />,
    );

  it("interpolates the integration label into the title", async () => {
    renderDialog();
    expect(await screen.findByText("Reset review watch?")).toBeTruthy();
  });

  it("shows the checking copy while the preview is in flight", async () => {
    // A promise that never settles keeps the dialog in the loading branch.
    renderDialog({ previewLoader: vi.fn().mockReturnValue(new Promise(() => {})) });
    expect(await screen.findByText(/checking how many tasks would be deleted/i)).toBeTruthy();
  });

  it("never renders a zero count before the preview resolves", async () => {
    renderDialog({ previewLoader: vi.fn().mockReturnValue(new Promise(() => {})) });
    await screen.findByText(/checking how many tasks would be deleted/i);
    expect(screen.queryByText(/delete 0 tasks/i)).toBeNull();
  });

  it("uses the zero-task copy when nothing was created", async () => {
    renderDialog();
    expect(await screen.findByText(/no tasks were created by this watch yet/i)).toBeTruthy();
  });

  it("uses the singular form for exactly one task", async () => {
    renderDialog({ previewLoader: vi.fn().mockResolvedValue({ taskCount: 1 }) });
    const description = await screen.findByTestId("reset-watch-dialog-description");
    expect(description.textContent).toContain("delete 1 task previously created");
    expect(description.textContent).not.toContain("1 tasks");
  });

  it("uses the plural form for more than one task", async () => {
    renderDialog({ previewLoader: vi.fn().mockResolvedValue({ taskCount: 4 }) });
    const description = await screen.findByTestId("reset-watch-dialog-description");
    expect(description.textContent).toContain("delete 4 tasks previously created");
  });

  it("falls back to the delete-all copy when a lenient preview fails", async () => {
    renderDialog({ previewLoader: vi.fn().mockRejectedValue(new Error("nope")) });
    expect(await screen.findByText(/delete every task previously created/i)).toBeTruthy();
  });

  it("asks the user to retry when a strict preview fails", async () => {
    renderDialog({
      requirePreviewSuccess: true,
      previewLoader: vi.fn().mockRejectedValue(new Error("nope")),
    });
    expect(await screen.findByText(/could not load the affected task count/i)).toBeTruthy();
  });
});
