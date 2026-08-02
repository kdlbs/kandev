import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StorageRunHistory } from "./storage-run-history";

afterEach(cleanup);

describe("StorageRunHistory", () => {
  it("renders an independent loading state", () => {
    render(<StorageRunHistory runs={[]} loading />);

    expect(screen.getByTestId("storage-run-history-spinner")).toBeTruthy();
    expect(screen.getByText("Loading maintenance history…")).toBeTruthy();
    expect(screen.queryByText("No storage maintenance runs yet.")).toBeNull();
  });

  it("renders an isolated error state", () => {
    render(<StorageRunHistory runs={[]} error="history unavailable" />);

    expect(screen.getByTestId("storage-run-history-error").textContent).toContain(
      "history unavailable",
    );
    expect(screen.queryByTestId("storage-run-history-spinner")).toBeNull();
  });
});
