import { useState } from "react";
import {
  cleanup,
  createEvent,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Dialog, DialogContent, DialogFooter, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { ApiError } from "@/lib/api/client";
import { NewSessionAdvancedSettings } from "./new-session-advanced-settings";
import type { NewSessionMCPPickerProps } from "./new-session-mcp-picker";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";

const viewport = vi.hoisted(() => ({ touch: false, mobile: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => viewport.touch }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: viewport.mobile }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({
      workspaces: { activeId: "ws" },
      agentProfiles: { items: [] },
      repositories: { itemsByWorkspaceId: {} },
      kanban: { tasks: [] },
      workflows: { items: [] },
      taskSessions: { items: {} },
    }),
}));

const MCP_TRIGGER = "new-session-mcp-select";
const ADVANCED_LABEL = "Advanced settings";

const definition: MCPServerDefinition = {
  id: "docs",
  workspace_id: "ws",
  display_name: "Documentation",
  runtime_name: "docs",
  normalized_runtime_name: "docs",
  enabled: true,
  execution_mode: "remote",
  transport: "sse",
  configuration: { url: "https://example.test/mcp" },
  source: "custom",
  revision: 1,
  created_at: "now",
  updated_at: "now",
};

function Form({
  onSubmit,
  ...props
}: Partial<NewSessionMCPPickerProps> & { onSubmit: () => void }) {
  const [selectedIds, onSelectedIdsChange] = useState<string[]>([]);
  const [portalContainer, setPortalContainer] = useState<HTMLDivElement | null>(null);
  return (
    <Dialog open>
      <DialogContent ref={setPortalContainer}>
        <DialogTitle>Launch</DialogTitle>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <NewSessionAdvancedSettings
            definitions={[definition]}
            selectedIds={selectedIds}
            onSelectedIdsChange={onSelectedIdsChange}
            portalContainer={portalContainer}
            {...props}
          />
          <DialogFooter>
            <Button type="submit">Launch agent</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function expandAndOpen() {
  fireEvent.click(screen.getByRole("button", { name: ADVANCED_LABEL }));
  fireEvent.click(screen.getByTestId(MCP_TRIGGER));
  return screen.getByTestId("new-session-mcp-options");
}

afterEach(() => {
  cleanup();
  viewport.touch = false;
  viewport.mobile = false;
});

// @covers AC-UI-NEW-SESSION-ADVANCED-001.4, AC-UI-NEW-SESSION-ADVANCED-002.1/2/4
describe("New session optional selections", () => {
  it("retains explicit additions across close and collapse without submitting", async () => {
    const onSubmit = vi.fn();
    render(
      <Form
        onSubmit={onSubmit}
        inherited={[
          {
            definition: { ...definition, id: "inherited", display_name: "Inherited documentation" },
            origins: [{ scope: "task", owner_id: "task", workspace_id: "ws" }],
          },
        ]}
      />,
    );
    expect(screen.queryByTestId(MCP_TRIGGER)).toBeNull();
    const picker = expandAndOpen();
    expect(screen.getByRole("button", { name: "MCP servers 0 selected" })).toBeTruthy();
    expect(within(picker).getByText("Inherited documentation")).toBeTruthy();
    fireEvent.click(within(picker).getByRole("checkbox", { name: "Documentation" }));
    fireEvent.keyDown(within(picker).getByRole("textbox"), { key: "Enter" });
    expect(onSubmit).not.toHaveBeenCalled();
    fireEvent.keyDown(within(picker).getByRole("textbox"), { key: "Escape" });
    await waitFor(() => expect(screen.queryByTestId("new-session-mcp-options")).toBeNull());
    expect(screen.getByText("1 selected")).toBeTruthy();
    expect(screen.getByRole("button", { name: "MCP servers 1 selected" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: ADVANCED_LABEL }));
    const reopened = expandAndOpen();
    expect(
      within(reopened)
        .getByRole("checkbox", { name: "Documentation" })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "preserves IME candidate confirmation for %j",
    (composition) => {
      const onSubmit = vi.fn();
      render(<Form onSubmit={onSubmit} />);
      const search = within(expandAndOpen()).getByRole("textbox");
      const confirmation = createEvent.keyDown(search, { key: "Enter", ...composition });
      fireEvent(search, confirmation);
      expect(confirmation.defaultPrevented).toBe(false);
      expect(onSubmit).not.toHaveBeenCalled();
      const enter = createEvent.keyDown(search, { key: "Enter" });
      fireEvent(search, enter);
      expect(enter.defaultPrevented).toBe(true);
      expect(onSubmit).not.toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button", { name: "Launch agent" }));
      expect(onSubmit).toHaveBeenCalledOnce();
    },
  );

  it.each(["coarse tablet", "fine-pointer phone"])("uses a sheet for %s", (mode) => {
    viewport.touch = mode === "coarse tablet";
    viewport.mobile = mode === "fine-pointer phone";
    render(<Form onSubmit={vi.fn()} />);
    const picker = expandAndOpen();
    expect(picker.getAttribute("data-slot")).toBe("drawer-content");
    expect(within(picker).getByRole("button", { name: "Close" })).toBeTruthy();
  });

  it("keeps loading and empty feedback inside the picker", () => {
    const { rerender } = render(<Form onSubmit={vi.fn()} definitionsLoading />);
    const picker = expandAndOpen();
    expect(within(picker).getByText("Loading MCP servers...")).toBeTruthy();
    rerender(<Form onSubmit={vi.fn()} definitions={[]} />);
    expect(within(picker).getByText("No MCP servers match this search.")).toBeTruthy();
  });

  it("closes the picker and disables its trigger when access becomes unavailable", async () => {
    const { rerender } = render(<Form onSubmit={vi.fn()} />);
    expandAndOpen();
    const error = new ApiError("Reload required", 409, {});
    error.handled = true;
    rerender(<Form onSubmit={vi.fn()} error={error} />);
    await waitFor(() => expect(screen.queryByTestId("new-session-mcp-options")).toBeNull());
    expect(screen.getByTestId(MCP_TRIGGER).hasAttribute("disabled")).toBe(true);
  });

  it("keeps disabled servers unselectable and disables the trigger while busy", () => {
    const { rerender } = render(
      <Form onSubmit={vi.fn()} definitions={[{ ...definition, enabled: false }]} />,
    );
    const picker = expandAndOpen();
    expect(
      within(picker).getByRole("checkbox", { name: "Documentation" }).hasAttribute("disabled"),
    ).toBe(true);
    rerender(<Form onSubmit={vi.fn()} disabled />);
    expect(screen.getByTestId(MCP_TRIGGER).hasAttribute("disabled")).toBe(true);
  });
});
