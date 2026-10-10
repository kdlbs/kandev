import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/repository-discovery-controls", () => ({
  RepositoryDiscoveryControls: (props: { workspaceId: string | null; enabled?: boolean }) => (
    <div data-testid="mock-discovery-controls">
      <span>{props.workspaceId}</span>
      <span>{String(props.enabled)}</span>
    </div>
  ),
}));

import { RepositoryDiscoveryDialog } from "./repository-discovery-dialog";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RepositoryDiscoveryDialog", () => {
  it("renders discovery controls inside the dialog when open", () => {
    const onOpenChange = vi.fn();
    render(<RepositoryDiscoveryDialog open onOpenChange={onOpenChange} workspaceId="ws-123" />);

    expect(screen.getByTestId("repository-discovery-dialog")).toBeTruthy();
    expect(screen.getByTestId("mock-discovery-controls")).toBeTruthy();
    expect(screen.getByText("ws-123")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "common:close" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("does not render dialog content when closed", () => {
    render(<RepositoryDiscoveryDialog open={false} onOpenChange={vi.fn()} workspaceId="ws-123" />);

    expect(screen.queryByTestId("repository-discovery-dialog")).toBeNull();
  });
});
