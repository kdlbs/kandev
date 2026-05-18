/**
 * Tests for TaskBody — the simple|advanced switcher and the URL-mode
 * resolver that decides which slot to mount.
 */

import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TaskBody, resolveTaskBodyMode } from "./TaskBody";

afterEach(cleanup);

describe("resolveTaskBodyMode", () => {
  it("returns the default when no overrides are set", () => {
    expect(resolveTaskBodyMode({}, "simple")).toBe("simple");
    expect(resolveTaskBodyMode({}, "advanced")).toBe("advanced");
    expect(resolveTaskBodyMode(null, "simple")).toBe("simple");
  });

  it("flag-style ?simple flips to simple even when default is advanced", () => {
    expect(resolveTaskBodyMode({ simple: "" }, "advanced")).toBe("simple");
  });

  it("flag-style ?advanced flips to advanced even when default is simple", () => {
    expect(resolveTaskBodyMode({ advanced: "" }, "simple")).toBe("advanced");
  });

  it("legacy ?mode=advanced still flips", () => {
    expect(resolveTaskBodyMode({ mode: "advanced" }, "simple")).toBe("advanced");
    expect(resolveTaskBodyMode({ mode: "simple" }, "advanced")).toBe("simple");
  });

  it("ignores unknown ?mode values and uses default", () => {
    expect(resolveTaskBodyMode({ mode: "foo" }, "advanced")).toBe("advanced");
  });

  it("works with URLSearchParams", () => {
    expect(resolveTaskBodyMode(new URLSearchParams("simple"), "advanced")).toBe("simple");
    expect(resolveTaskBodyMode(new URLSearchParams("advanced"), "simple")).toBe("advanced");
    expect(resolveTaskBodyMode(new URLSearchParams(""), "simple")).toBe("simple");
  });
});

describe("TaskBody", () => {
  it("renders the simple slot in simple mode", () => {
    render(
      <TaskBody
        mode="simple"
        simpleSlot={<div>simple-pane</div>}
        advancedSlot={<div>advanced-pane</div>}
      />,
    );
    expect(screen.getByText("simple-pane")).toBeTruthy();
    expect(screen.queryByText("advanced-pane")).toBeNull();
  });

  it("renders the advanced slot in advanced mode", () => {
    render(
      <TaskBody
        mode="advanced"
        simpleSlot={<div>simple-pane</div>}
        advancedSlot={<div>advanced-pane</div>}
      />,
    );
    expect(screen.getByText("advanced-pane")).toBeTruthy();
    expect(screen.queryByText("simple-pane")).toBeNull();
  });

  it("renders nothing when the chosen slot is missing", () => {
    const { container } = render(<TaskBody mode="simple" />);
    expect(container.textContent).toBe("");
  });
});
