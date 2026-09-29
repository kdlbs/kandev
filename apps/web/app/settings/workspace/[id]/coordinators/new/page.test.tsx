import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

let phase2 = false;
let scopes: string[] = ["workspace.manage"];

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => phase2 }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ workspaces: { items: [{ id: "w1", scopes }] } }),
}));
vi.mock("@/components/coordinators/setup/coordinator-setup", () => ({
  CoordinatorSetup: () => <div data-testid="setup" />,
}));
vi.mock("@/components/coordinators/coordinator-add-page", () => ({
  CoordinatorAddPage: () => <div data-testid="phase-1-form" />,
}));

import NewCoordinatorPage from "./page";

afterEach(() => {
  cleanup();
  phase2 = false;
  scopes = ["workspace.manage"];
});

describe("NewCoordinatorPage", () => {
  it("renders the phase-1 form while phase 2 is off", () => {
    render(<NewCoordinatorPage workspaceId="w1" />);
    expect(screen.getByTestId("phase-1-form")).toBeDefined();
  });

  it("renders the guided setup while phase 2 is on", () => {
    phase2 = true;
    render(<NewCoordinatorPage workspaceId="w1" />);
    expect(screen.getByTestId("setup")).toBeDefined();
  });

  it("keeps the phase-1 reader state for a reader", () => {
    phase2 = true;
    scopes = [];
    render(<NewCoordinatorPage workspaceId="w1" />);
    expect(screen.getByTestId("phase-1-form")).toBeDefined();
  });
});
