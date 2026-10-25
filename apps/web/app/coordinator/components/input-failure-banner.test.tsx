import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CoordinatorInputStatus } from "@/app/coordinator/use-coordinator-attention";
import { InputFailureBanner } from "./input-failure-banner";

afterEach(cleanup);

describe("InputFailureBanner", () => {
  it("renders nothing when no input has failed", () => {
    const inputs: CoordinatorInputStatus[] = [
      { kind: "tasks", error: false, loadedAt: 1 },
      { kind: "stalls", error: false, loadedAt: 1 },
      { kind: "proposals", error: false, loadedAt: 1 },
    ];
    const { container } = render(<InputFailureBanner inputs={inputs} retry={vi.fn()} />);
    expect(container.firstChild).toBeNull();
  });

  it("shows one line per failed input, with its own load time", () => {
    const loadedAt = new Date(2026, 8, 27, 8, 5).getTime();
    const inputs: CoordinatorInputStatus[] = [
      { kind: "tasks", error: true, loadedAt },
      { kind: "stalls", error: false, loadedAt },
      { kind: "proposals", error: true, loadedAt: undefined },
    ];
    render(<InputFailureBanner inputs={inputs} retry={vi.fn()} />);
    expect(
      screen.getByText(`Could not load this workspace's tasks. Showing what was loaded at 08:05.`),
    ).not.toBeNull();
    expect(screen.getByText("Could not load proposals.")).not.toBeNull();
    expect(screen.queryByText(/stall records/)).toBeNull();
  });

  it("calls retry on Try again", () => {
    const retry = vi.fn();
    render(
      <InputFailureBanner
        inputs={[{ kind: "tasks", error: true, loadedAt: undefined }]}
        retry={retry}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
