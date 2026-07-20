import { describe, expect, it } from "vitest";
import { readKanbanActiveWorkspaceCookie } from "./kanban-active-workspace-cookie";

function cookieStore(values: Record<string, string>) {
  return {
    get: (name: string) => {
      const value = values[name];
      return value ? { value } : undefined;
    },
  };
}

describe("readKanbanActiveWorkspaceCookie", () => {
  it("prefers the general active workspace cookie over the legacy office cookie", () => {
    expect(
      readKanbanActiveWorkspaceCookie(
        cookieStore({
          "kandev-active-workspace": "workspace-1",
          "office-active-workspace": "workspace-2",
        }),
      ),
    ).toBe("workspace-1");
  });

  it("falls back to the legacy office cookie", () => {
    expect(
      readKanbanActiveWorkspaceCookie(cookieStore({ "office-active-workspace": "workspace-2" })),
    ).toBe("workspace-2");
  });
});
