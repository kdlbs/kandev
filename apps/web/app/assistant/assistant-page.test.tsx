import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { AssistantPage } from "./assistant-page";
const fixture = vi.hoisted(() => ({ enabled: true, owner: "owner", mobile: false, load: vi.fn() }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (value: unknown) => unknown) =>
    select({
      features: { personalAssistant: fixture.enabled },
      auth: { user: { id: fixture.owner } },
      workspaces: { items: [{ id: "ws", name: "Example workspace" }] },
    }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: fixture.mobile }),
}));
vi.mock("@/hooks/domains/orchestration/use-assistant", () => ({ useAssistant: fixture.load }));
vi.mock("@/components/page-shell", () => ({
  PageShell: ({ children }: { children: ReactNode }) => <main>{children}</main>,
}));
vi.mock("./assistant-chat", () => ({
  AssistantChat: () => <textarea aria-label="Example draft" />,
}));
vi.mock("./assistant-controls", () => ({
  AssistantControls: () => <button>Example control</button>,
}));
vi.mock("./assistant-details", () => ({ AssistantDetails: () => <p>Example details</p> }));
vi.mock("./assistant-attention", () => ({ AssistantAttentionPanel: () => <p>Example input</p> }));
vi.mock("./assistant-setup", () => ({ AssistantSetup: () => <p>Select an existing assistant</p> }));
const binding = { id: "binding", owner_user_id: "owner", home_workspace_id: "ws", version: 1 };
beforeEach(() => {
  fixture.enabled = true;
  fixture.mobile = false;
  fixture.owner = "owner";
  fixture.load.mockReset().mockReturnValue({ binding, revision: 1, refresh: vi.fn() });
});
afterEach(cleanup);
it("never mounts the private reader while disabled and distinguishes setup from failure", () => {
  fixture.enabled = false;
  const view = render(<AssistantPage />);
  expect(fixture.load).not.toHaveBeenCalled();
  fixture.enabled = true;
  fixture.load.mockReturnValue({ binding: null });
  view.rerender(<AssistantPage />);
  expect(screen.getByText("Select an existing assistant")).toBeTruthy();
  fixture.load.mockReturnValue({ error: new Error("offline"), refresh: vi.fn() });
  view.rerender(<AssistantPage />);
  expect(screen.getByRole("alert")).toBeTruthy();
  expect(screen.queryByText("Select an existing assistant")).toBeNull();
});
it("preserves a draft through transient failure and mobile tabs, then clears it on owner change", () => {
  fixture.mobile = true;
  const view = render(<AssistantPage />);
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "Summarize the sample tasks." },
  });
  fireEvent.click(screen.getByRole("tab", { name: "Attention" }));
  expect(screen.getByRole("tabpanel").textContent).toContain("Example input");
  fireEvent.keyDown(screen.getByRole("tab", { name: "Attention" }), { key: "Home" });
  expect(screen.getByRole<HTMLTextAreaElement>("textbox").value).toBe(
    "Summarize the sample tasks.",
  );
  fixture.load.mockReturnValue({ binding, error: new Error("offline"), refresh: vi.fn() });
  view.rerender(<AssistantPage />);
  expect(screen.getByRole<HTMLTextAreaElement>("textbox").value).toBe(
    "Summarize the sample tasks.",
  );
  expect(
    screen.getByRole("button", { name: "Example control" }).closest("fieldset")?.disabled,
  ).toBe(true);
  expect(document.querySelectorAll("#assistant-panel-chat")).toHaveLength(1);
  fixture.owner = "another-owner";
  fixture.load.mockReturnValue({
    binding: { ...binding, owner_user_id: fixture.owner },
    refresh: vi.fn(),
  });
  view.rerender(<AssistantPage />);
  expect(screen.getByRole<HTMLTextAreaElement>("textbox").value).toBe("");
});
