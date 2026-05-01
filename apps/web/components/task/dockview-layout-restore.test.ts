import { describe, it, expect } from "vitest";
import { sanitizeLayout } from "./dockview-layout-restore";

const VALID_COMPONENTS = new Set<string>(["chat", "files", "shell", "git"]);

/**
 * Build a minimal valid SerializedDockview-shaped object — matches what
 * dockview's api.toJSON() produces for a 3-column layout (sidebar | center | right).
 */
function buildLayout(opts?: { centerSize?: number; sidebarSize?: number; rightSize?: number }) {
  return {
    grid: {
      root: {
        type: "branch" as const,
        size: 600,
        data: [
          {
            type: "leaf" as const,
            size: opts?.sidebarSize ?? 350,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          {
            type: "leaf" as const,
            size: opts?.centerSize ?? 800,
            data: { id: "g-center", views: ["chat"], activeView: "chat" },
          },
          {
            type: "leaf" as const,
            size: opts?.rightSize ?? 450,
            data: { id: "g-right", views: ["git", "shell"], activeView: "git" },
          },
        ],
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL" as const,
    },
    panels: {
      files: { id: "files", contentComponent: "files" },
      chat: { id: "chat", contentComponent: "chat" },
      git: { id: "git", contentComponent: "git" },
      shell: { id: "shell", contentComponent: "shell" },
    },
    activeGroup: "g-center",
  };
}

describe("sanitizeLayout - size validation", () => {
  it("returns the layout unchanged when all sizes are positive", () => {
    const layout = buildLayout();
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).not.toBeNull();
    expect(result.grid.root.data).toHaveLength(3);
  });

  it("returns null when a leaf node has size 0", () => {
    const layout = buildLayout({ centerSize: 0 });
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when a leaf node has negative size", () => {
    const layout = buildLayout({ sidebarSize: -50 });
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when a branch node has size 0", () => {
    const layout = buildLayout();
    layout.grid.root.size = 0;
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when nested branch has invalid size", () => {
    const layout = {
      grid: {
        root: {
          type: "branch" as const,
          size: 800,
          data: [
            {
              type: "leaf" as const,
              size: 350,
              data: { id: "g-sidebar", views: ["files"], activeView: "files" },
            },
            {
              type: "branch" as const,
              size: 0,
              data: [
                {
                  type: "leaf" as const,
                  size: 800,
                  data: { id: "g-center", views: ["chat"], activeView: "chat" },
                },
              ],
            },
          ],
        },
        height: 600,
        width: 1600,
        orientation: "HORIZONTAL" as const,
      },
      panels: {
        files: { id: "files", contentComponent: "files" },
        chat: { id: "chat", contentComponent: "chat" },
      },
      activeGroup: "g-center",
    };
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when grid.width is 0", () => {
    const layout = buildLayout();
    layout.grid.width = 0;
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when grid.height is 0", () => {
    const layout = buildLayout();
    layout.grid.height = 0;
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("preserves existing component-validation behavior alongside size checks", () => {
    const layout = buildLayout();
    // Add an unknown panel — sanitizer should remove it but keep the rest.
    layout.panels = {
      ...layout.panels,
      // @ts-expect-error - injecting an extra unknown panel
      unknown: { id: "unknown", contentComponent: "unknown-component" },
    };
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).not.toBeNull();
    expect(Object.keys(result.panels)).not.toContain("unknown");
  });
});
