import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { UnitsPage } from "./units-page";

const createUnit = vi.fn();
const deleteUnit = vi.fn();

vi.mock("@/lib/api/domains/org-units-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/org-units-api")>(
    "@/lib/api/domains/org-units-api",
  );
  const base = { org_id: "o", created_at: "", updated_at: "" };
  const fixture = [
    { ...base, id: "root", parent_id: "", kind: "root", name: "Acme", path: "/root/" },
    {
      ...base,
      id: "dept",
      parent_id: "root",
      kind: "standard",
      name: "Platform",
      path: "/root/dept/",
    },
    {
      ...base,
      id: "personal",
      parent_id: "root",
      kind: "personal",
      owner_user_id: "ada",
      name: "Ada",
      path: "/root/personal/",
    },
  ];
  return {
    ...actual,
    listUnits: vi.fn().mockResolvedValue({ units: fixture, total: fixture.length }),
    listUnitMembers: vi.fn().mockResolvedValue({ members: [], total: 0 }),
    createUnit: (...args: unknown[]) => createUnit(...args),
    deleteUnit: (...args: unknown[]) => deleteUnit(...args),
  };
});

vi.mock("@/lib/api/domains/team-access-api", () => ({
  listDirectoryUsers: vi.fn().mockResolvedValue({ users: [], total: 0 }),
  listWorkspaceMembers: vi.fn().mockResolvedValue({ members: [], total: 0 }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

async function renderTree() {
  render(<UnitsPage />);
  await waitFor(() => expect(screen.getAllByTestId("unit-row")).toHaveLength(3));
  return screen.getAllByTestId("unit-row");
}

describe("UnitsPage", () => {
  it("renders the tree with a row per unit", async () => {
    await renderTree();
    expect(screen.getByText("Platform")).toBeTruthy();
  });

  // A personal unit takes no members and cannot be deleted, so offering those
  // controls would only produce refusals the person cannot act on.
  it("offers no member or delete control on a personal unit", async () => {
    const rows = await renderTree();
    const personal = rows[2]!;
    expect(within(personal).queryByTestId("unit-members")).toBeNull();
    expect(within(personal).queryByTestId("unit-delete")).toBeNull();
    expect(within(personal).queryByTestId("unit-add-child")).toBeNull();
  });

  // The root is the whole organization; deleting it would leave every
  // workspace beneath it homeless.
  it("offers no delete control on the root", async () => {
    const rows = await renderTree();
    expect(within(rows[0]!).queryByTestId("unit-delete")).toBeNull();
    expect(within(rows[0]!).getByTestId("unit-add-child")).toBeTruthy();
  });

  it("creates a child unit under the chosen parent", async () => {
    const rows = await renderTree();
    fireEvent.click(within(rows[1]!).getByTestId("unit-add-child"));
    fireEvent.change(await screen.findByTestId("unit-name-input"), {
      target: { value: "Runtime" },
    });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(createUnit).toHaveBeenCalledWith("dept", "Runtime"));
  });
});
