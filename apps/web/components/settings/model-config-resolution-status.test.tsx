import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ModelConfigResolutionStatus } from "./model-config-resolution-status";

afterEach(cleanup);

describe("ModelConfigResolutionStatus", () => {
  it("shows a localized loading state", () => {
    render(
      <ModelConfigResolutionStatus
        status="probing"
        error={null}
        isLoading
        onRetry={vi.fn(async () => {})}
      />,
    );

    expect(screen.getByTestId("model-config-resolution-loading")).toBeTruthy();
    expect(screen.getByText("Loading model options…")).toBeTruthy();
  });

  it("shows a localized failure and retries resolution", () => {
    const onRetry = vi.fn(async () => {});
    render(
      <ModelConfigResolutionStatus
        status="failed"
        error="raw provider stderr"
        isLoading={false}
        onRetry={onRetry}
      />,
    );

    expect(screen.getByTestId("model-config-resolution-error")).toBeTruthy();
    expect(screen.getByText("Model options could not be loaded.")).toBeTruthy();
    expect(screen.queryByText("raw provider stderr")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });
});
