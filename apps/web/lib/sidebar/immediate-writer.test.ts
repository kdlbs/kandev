import { describe, it, expect, vi } from "vitest";
import { createSidebarWriter } from "./immediate-writer";
import { defaultSidebarLayout, type SidebarLayout } from "./layout-types";
import { toggleNodeVisibility, moveSection } from "./layout-operations";

describe("immediate sidebar writes", () => {
  it("serializes edits using each acknowledged revision and isolates workspaces", async () => {
    const layouts: Record<string, SidebarLayout> = {
      a: defaultSidebarLayout(),
      b: defaultSidebarLayout(),
    };
    const write = vi.fn(async (workspace: string, layout: SidebarLayout) => {
      const saved = { ...layout, revision: layout.revision + 1 };
      layouts[workspace] = saved;
    });
    const writer = createSidebarWriter({ read: (id) => layouts[id], write });
    await Promise.all([
      writer("a", (layout) => toggleNodeVisibility(layout, "home", false)),
      writer("a", (layout) => moveSection(layout, "integrations", 0)),
      writer("b", (layout) => toggleNodeVisibility(layout, "canvases", false)),
    ]);
    expect(write.mock.calls.map(([id, layout]) => [id, layout.revision])).toEqual([
      ["a", 0],
      ["a", 1],
      ["b", 0],
    ]);
    expect(layouts.a.nodes.find((node) => node.id === "home")?.visible).toBe(false);
    expect(layouts.a.nodes[0].id).toBe("integrations");
    expect(layouts.b.nodes.find((node) => node.id === "home")?.visible).toBe(true);
  });
  it("does not publish failed edits and accepts a subsequent operation", async () => {
    let saved = defaultSidebarLayout();
    const write = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockImplementation(async (_id, next) => {
        saved = next;
      });
    const writer = createSidebarWriter({ read: () => saved, write });
    await expect(
      writer("a", (layout) => toggleNodeVisibility(layout, "home", false)),
    ).rejects.toThrow("offline");
    await writer("a", (layout) => toggleNodeVisibility(layout, "integrations", false));
    expect(saved.nodes.find((node) => node.id === "home")?.visible).toBe(true);
  });
});
