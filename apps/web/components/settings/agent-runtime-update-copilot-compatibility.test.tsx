import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { AgentRuntimeUpdateControl } from "./agent-runtime-update-control";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children?: ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children?: ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children, ...props }: { children?: ReactNode } & Record<string, unknown>) => (
    <div {...props}>{children}</div>
  ),
  DialogDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DrawerContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DrawerDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DrawerFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DrawerHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

const AGENT_NAME = "copilot-acp";
const PACKAGE_NAME = "@github/copilot";
const UNCHECKED_PREVIEW_STATE = "not_checked" as const;
const UNKNOWN_RUNTIME_VERSION = "9.9.9";
const COMPATIBILITY_STATE_ATTRIBUTE = "data-state";

function preview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return {
    update_mode: "pinned",
    agent_name: AGENT_NAME,
    package: PACKAGE_NAME,
    current_version: "1.0.104",
    default_version: "1.0.88",
    effective_version: "1.0.104",
    active_version: "1.0.104",
    target_version: UNKNOWN_RUNTIME_VERSION,
    operation: "update",
    available_versions: [
      { version: UNKNOWN_RUNTIME_VERSION, latest: true },
      { version: "1.0.104", latest: false },
      { version: "1.0.88", latest: false },
    ],
    command: ["npm", "exec"],
    command_string: "npm exec",
    ...overrides,
  };
}

function uncheckedPreview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return preview({
    copilot_plugin_compatibility: {
      runtime_version: UNKNOWN_RUNTIME_VERSION,
      state: UNCHECKED_PREVIEW_STATE,
      selected_profile_count: 2,
    },
    ...overrides,
  });
}

function renderControl(onPreview: () => Promise<AgentUpdatePreview>) {
  const onUpdate = vi.fn().mockResolvedValue({
    job_id: "job-1",
    update_mode: "pinned",
    agent_name: AGENT_NAME,
    status: "queued",
    started_at: "2026-01-01T00:00:00.000Z",
  } satisfies AgentUpdateJob);
  render(
    <AgentRuntimeUpdateControl
      agentName={AGENT_NAME}
      displayName="Copilot"
      runtimeUpdate={{
        supported: true,
        update_mode: "pinned",
        package: PACKAGE_NAME,
        default_version: "1.0.88",
        effective_version: "1.0.104",
      }}
      onPreview={onPreview}
      onUpdate={onUpdate}
    />,
  );
  return onUpdate;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("Copilot runtime plugin compatibility preview", () => {
  it("leaves unknown targets unchecked and reports profile selections before approval", async () => {
    const onUpdate = renderControl(() => Promise.resolve(uncheckedPreview()));
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));

    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.getAttribute(COMPATIBILITY_STATE_ATTRIBUTE)).toBe(UNCHECKED_PREVIEW_STATE);
    expect(notice.textContent).toContain(`Copilot ${UNKNOWN_RUNTIME_VERSION}`);
    expect(notice.textContent).toContain(
      `Plugin compatibility for Copilot ${UNKNOWN_RUNTIME_VERSION}: unchecked.`,
    );
    expect(notice.textContent).toContain("2 saved profiles select plugins.");
    expect(notice.textContent).not.toMatch(/qualified versions|will block/i);
    const confirm = screen.getByTestId(`agent-update-confirm-${AGENT_NAME}`);
    expect(confirm.hasAttribute("disabled")).toBe(false);
    fireEvent.click(confirm);
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(AGENT_NAME, UNKNOWN_RUNTIME_VERSION));
  });

  it("shows unknown impact instead of zero when the profile read failed", async () => {
    renderControl(() =>
      Promise.resolve(
        uncheckedPreview({
          copilot_plugin_compatibility: {
            runtime_version: UNKNOWN_RUNTIME_VERSION,
            state: UNCHECKED_PREVIEW_STATE,
            selected_profile_count: null,
          },
        }),
      ),
    );
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.textContent).toContain("Saved profile selections could not be counted.");
    expect(notice.textContent).not.toContain("No saved profiles select plugins.");
  });

  it("shows zero profile selections without deriving a target limitation", async () => {
    renderControl(() =>
      Promise.resolve(
        uncheckedPreview({
          copilot_plugin_compatibility: {
            runtime_version: UNKNOWN_RUNTIME_VERSION,
            state: UNCHECKED_PREVIEW_STATE,
            selected_profile_count: 0,
          },
        }),
      ),
    );
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.textContent).toContain("No saved profiles select plugins.");
    expect(notice.textContent).toContain(
      `Plugin compatibility for Copilot ${UNKNOWN_RUNTIME_VERSION}: unchecked.`,
    );
  });

  it("renders a positive capability observation without promising every selected plugin can load", async () => {
    renderControl(() =>
      Promise.resolve(
        preview({
          copilot_plugin_compatibility: {
            runtime_version: "1.0.104",
            state: "compatible",
            selected_profile_count: 2,
          },
        }),
      ),
    );
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.getAttribute(COMPATIBILITY_STATE_ATTRIBUTE)).toBe("compatible");
    expect(notice.textContent).toContain(
      "Copilot 1.0.104 advertises profile plugin loading. Selected plugins still depend on their own installation and native policy.",
    );
    expect(notice.textContent).not.toMatch(/qualified versions/i);
  });

  it("treats an omitted compatibility state as unchecked for an older Copilot server", async () => {
    renderControl(() => Promise.resolve(preview()));
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    expect(notice.textContent).toContain(
      "Plugin compatibility for Copilot Unknown: unchecked. It is checked when a profile is used.",
    );
    expect(notice.getAttribute(COMPATIBILITY_STATE_ATTRIBUTE)).toBe(UNCHECKED_PREVIEW_STATE);
  });
});

it("labels a negative capability observation without claiming selected roots were evaluated", async () => {
  renderControl(() =>
    Promise.resolve(
      preview({
        copilot_plugin_compatibility: {
          runtime_version: UNKNOWN_RUNTIME_VERSION,
          state: "incompatible",
          selected_profile_count: 2,
        },
      }),
    ),
  );
  fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
  const notice = await screen.findByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
  expect(notice.getAttribute(COMPATIBILITY_STATE_ATTRIBUTE)).toBe("incompatible");
  expect(notice.textContent).toContain(
    `Copilot ${UNKNOWN_RUNTIME_VERSION} does not advertise profile plugin loading.`,
  );
  expect(notice.textContent).toContain("This preview does not evaluate selected roots");
  expect(notice.textContent).not.toMatch(/will block/i);
});
