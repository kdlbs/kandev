import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { WorkspacePlacementCard } from "./workspace-placement-card";

const placeWorkspace = vi.fn();

vi.mock("@/lib/api/domains/org-units-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/org-units-api")>(
    "@/lib/api/domains/org-units-api",
  );
  const base = { org_id: "o", created_at: "", updated_at: "" };
  return {
    ...actual,
    listUnits: vi.fn().mockResolvedValue({
      units: [
        { ...base, id: "root", parent_id: "", kind: "root", name: "Acme", path: "/root/" },
        {
          ...base,
          id: "dept",
          parent_id: "root",
          kind: "standard",
          name: "Platform",
          path: "/root/dept/",
        },
      ],
      total: 2,
    }),
    placeWorkspace: (...args: unknown[]) => placeWorkspace(...args),
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const OWNER = ["workspace.read", "workspace.manage"];
const PICKER = "workspace-placement-picker";

describe("WorkspacePlacementCard", () => {
  it("moves the workspace to the chosen unit", async () => {
    placeWorkspace.mockResolvedValue({ id: "ws-1", unit_id: "dept" });
    render(<WorkspacePlacementCard workspaceId="ws-1" unitId="root" scopes={OWNER} />);

    fireEvent.click(await screen.findByTestId(PICKER));
    fireEvent.click(await screen.findByText(/Platform/));

    await waitFor(() => expect(placeWorkspace).toHaveBeenCalledWith("ws-1", "dept"));
  });

  // Moving a workspace changes who reaches it, so it belongs to whoever can
  // manage the workspace and nobody else.
  it("disables the picker without workspace.manage", async () => {
    render(<WorkspacePlacementCard workspaceId="ws-1" unitId="root" scopes={["workspace.read"]} />);

    const picker = await screen.findByTestId(PICKER);
    expect(picker.hasAttribute("disabled")).toBe(true);
  });

  it("restores the previous unit when the move is refused", async () => {
    placeWorkspace.mockRejectedValue(new Error("that unit belongs to another organization"));
    render(<WorkspacePlacementCard workspaceId="ws-1" unitId="root" scopes={OWNER} />);

    fireEvent.click(await screen.findByTestId(PICKER));
    fireEvent.click(await screen.findByText(/Platform/));

    await waitFor(() => expect(placeWorkspace).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByTestId(PICKER).textContent).toContain("Acme"));
  });
});
