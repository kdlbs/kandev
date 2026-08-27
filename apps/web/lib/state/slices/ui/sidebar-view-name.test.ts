import { describe, expect, it } from "vitest";

import { DEFAULT_VIEW, createDefaultSidebarView, sidebarViewName } from "./sidebar-view-builtins";

const translate = (key: string) => `«${key}»`;

/**
 * `SidebarView.name` is persisted AND user-editable, so the built-in's display
 * name resolves through the catalog while the stored value stays canonical
 * English. Keying only off the id would have shown the translated default over
 * a name the user typed.
 */
describe("sidebarViewName", () => {
  it("localizes the built-in view while it keeps its canonical name", () => {
    expect(sidebarViewName(DEFAULT_VIEW, translate)).toBe("«sidebar:viewAllTasks»");
  });

  it("keeps a renamed built-in view showing the user's name", () => {
    const renamed = { ...DEFAULT_VIEW, name: "My stuff" };

    expect(sidebarViewName(renamed, translate)).toBe("My stuff");
  });

  it("never localizes a user-created view", () => {
    const custom = createDefaultSidebarView("view-abc", "Blocked work");

    expect(sidebarViewName(custom, translate)).toBe("Blocked work");
  });

  it("keeps the stored default name canonical English", () => {
    // What sync writes to the server; translating it would freeze a locale in.
    expect(DEFAULT_VIEW.name).toBe("All tasks");
  });
});
