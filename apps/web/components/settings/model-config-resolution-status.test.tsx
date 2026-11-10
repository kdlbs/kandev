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
    expect(screen.getByTestId("model-config-resolution-error").className).not.toContain(
      "flex-wrap",
    );
    expect(screen.getByRole("button", { name: "Retry" }).className).toContain("shrink-0");

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("shows policy recovery inline and falls back for an unknown reason", () => {
    const { rerender } = render(
      <ModelConfigResolutionStatus
        status="unsupported"
        reasonCode="policy_unverified"
        error="model option resolution failed"
        isLoading={false}
        onRetry={vi.fn(async () => {})}
      />,
    );

    expect(screen.getByTestId("model-config-resolution-error").textContent).toContain(
      "Model options are blocked by this plugin selection. Uncheck plugins, Save, then retry model options.",
    );
    expect(screen.queryByText("Model options could not be loaded.")).toBeNull();

    rerender(
      <ModelConfigResolutionStatus
        status="unsupported"
        reasonCode="native_output"
        error="model option resolution failed"
        isLoading={false}
        onRetry={vi.fn(async () => {})}
      />,
    );
    expect(screen.getByText("Model options could not be loaded.")).toBeTruthy();
  });
});
