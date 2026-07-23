import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { WorkspaceUnavailable } from "./workspace-unavailable";

afterEach(cleanup);

describe("WorkspaceUnavailable", () => {
  it("keeps the raw session error behind a collapsed disclosure", () => {
    render(
      <WorkspaceUnavailable error="fatal: unable to access github.com: Could not resolve host" />,
    );

    expect(screen.getByText("Workspace unavailable")).toBeTruthy();
    expect(screen.queryByText("Session failed")).toBeNull();
    const details = screen.getByText("Technical details").closest("details");
    expect(details?.open).toBe(false);

    fireEvent.click(screen.getByText("Technical details"));
    expect(details?.open).toBe(true);
    const error = screen.getByText(/Could not resolve host/);
    expect(error).toBeTruthy();
    expect(error.className).toContain("max-h-48");
    expect(error.className).toContain("overflow-y-auto");
  });
});
