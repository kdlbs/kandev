import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { SessionMobileBottomNav } from "./session-mobile-bottom-nav";

const PLUGIN_A = "plugin-a";
const PLUGIN_B = "plugin-b";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_A);
  pluginRegistry.unregisterPlugin(PLUGIN_B);
});

describe("SessionMobileBottomNav", () => {
  it("offers a touch-sized review route for linked merge requests", () => {
    const onPanelChange = vi.fn();
    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={onPanelChange}
        hasReview
        showStatus
        onOpenStatus={vi.fn()}
      />,
    );

    const review = screen.getByRole("button", { name: "Review" });
    expect(review.className).toContain("min-h-11");
    fireEvent.click(review);
    expect(onPanelChange).toHaveBeenCalledWith("review");
  });

  it("does not consume navigation space without a linked merge request", () => {
    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={vi.fn()}
        showStatus
        onOpenStatus={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: "Review" })).toBeNull();
  });

  it("opens Status as an action without changing the selected mobile panel", () => {
    const onPanelChange = vi.fn();
    const onOpenStatus = vi.fn();

    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={onPanelChange}
        showStatus
        onOpenStatus={onOpenStatus}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Status" }));

    expect(onOpenStatus).toHaveBeenCalledOnce();
    expect(onPanelChange).not.toHaveBeenCalled();
  });

  it("describes an active connectivity warning on the Status action", () => {
    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={vi.fn()}
        showStatus
        onOpenStatus={vi.fn()}
        connectionIssueSeverity="lost"
      />,
    );

    const status = screen.getByRole("button", {
      name: "Connection lost for at least 10 seconds. Live updates may be stale.",
    });
    expect(status.getAttribute("data-connection-severity")).toBe("lost");
  });

  it("does not reserve navigation space when Status is disabled", () => {
    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={vi.fn()}
        showStatus={false}
        onOpenStatus={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Status" })).toBeNull();
  });
});

describe("SessionMobileBottomNav plugin panels", () => {
  it("groups multiple mobile plugin panels behind one bounded Panels action", () => {
    function Notes() {
      return null;
    }
    pluginRegistry
      .forPlugin(PLUGIN_A)
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });
    pluginRegistry
      .forPlugin(PLUGIN_B)
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });
    const onPanelChange = vi.fn();

    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={onPanelChange}
        showStatus={false}
        onOpenStatus={vi.fn()}
      />,
    );

    expect(screen.getAllByRole("button", { name: "Panels" })).toHaveLength(1);
    expect(screen.queryAllByRole("button", { name: "Notes" })).toHaveLength(0);

    fireEvent.click(screen.getByRole("button", { name: "Panels" }));

    const options = screen.getAllByTestId(/^mobile-plugin-panel-option-/);
    expect(options).toHaveLength(2);
    expect(options[0]?.className).toContain("min-h-11");
    expect(options[1]?.className).toContain("min-h-11");

    fireEvent.click(options[1]!);
    expect(onPanelChange).toHaveBeenLastCalledWith("plugin:plugin-b:notes");
    expect(screen.queryByTestId("mobile-plugin-panel-option-plugin-b-notes")).toBeNull();
  });

  it("keeps the grouped Panels action active for a selected plugin panel", () => {
    function Notes() {
      return null;
    }
    pluginRegistry
      .forPlugin(PLUGIN_A)
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });

    render(
      <SessionMobileBottomNav
        activePanel="plugin:plugin-a:notes"
        onPanelChange={vi.fn()}
        showStatus={false}
        onOpenStatus={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Panels" }).className).toContain("text-primary");
  });

  it("keeps the grouped Panels action active for Prompt history", () => {
    render(
      <SessionMobileBottomNav
        activePanel="prompt-history"
        onPanelChange={vi.fn()}
        showPromptHistory
        showStatus={false}
        onOpenStatus={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Panels" }).className).toContain("text-primary");
  });
});

describe("SessionMobileBottomNav panel picker", () => {
  it("offers Prompt history through the grouped Panels action", () => {
    const onPanelChange = vi.fn();

    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={onPanelChange}
        showPromptHistory
        showStatus={false}
        onOpenStatus={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Panels" }));
    fireEvent.click(screen.getByTestId("mobile-prompt-history-option"));

    expect(onPanelChange).toHaveBeenCalledWith("prompt-history");
  });

  it("offers applicable task canvases in the native picker", () => {
    const onOpenCanvas = vi.fn();
    const taskCanvas: Canvas = {
      id: "canvas-1",
      plugin_instance_id: "instance-1",
      plugin_id: "plugin-1",
      workspace_id: "workspace-1",
      task_id: "task-1",
      scope_kind: "task",
      title: "Release board",
      status: "active",
      active_release_id: "release-1",
      active_release_status: "valid",
    };

    render(
      <SessionMobileBottomNav
        activePanel="chat"
        onPanelChange={vi.fn()}
        showStatus={false}
        onOpenStatus={vi.fn()}
        taskCanvases={[taskCanvas]}
        onOpenCanvas={onOpenCanvas}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Panels" }));
    const option = screen.getByTestId("mobile-canvas-option-canvas-1");
    expect(option.className).toContain("min-h-11");
    fireEvent.click(option);

    expect(onOpenCanvas).toHaveBeenCalledWith("canvas-1");
  });
});
