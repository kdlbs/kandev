import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KubernetesSessionsCard } from "./kubernetes-sessions-card";

const responsive = vi.hoisted(() => ({ isMobile: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: responsive.isMobile }),
}));

afterEach(() => {
  cleanup();
  responsive.isMobile = false;
});

describe("KubernetesSessionsCard", () => {
  it("keeps retained pods without sessions distinct on both layouts", () => {
    responsive.isMobile = false;
    const state = sessionsState();
    state.sessions = ["task-first", "task-second"].map((task) => ({
      ...state.sessions[0],
      task_id: task,
      session_id: "",
      pod_name: task,
    }));
    const rendered = render(<KubernetesSessionsCard state={state} />);
    expect(
      screen
        .getAllByTestId("kubernetes-session-task-link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/t/task-first", "/t/task-second"]);
    fireEvent.click(screen.getAllByRole("button", { name: "Show session details" })[0]);
    expect(screen.getAllByTestId("kubernetes-session-details-row")).toHaveLength(1);
    state.sessions = state.sessions.slice(1);
    rendered.rerender(<KubernetesSessionsCard state={state} />);
    expect(screen.getAllByTestId("kubernetes-session-row")).toHaveLength(1);
    responsive.isMobile = true;
    rendered.rerender(<KubernetesSessionsCard state={state} />);
    expect(screen.getByTestId("kubernetes-session-task-link").getAttribute("href")).toBe(
      "/t/task-second",
    );
    expect(screen.getByTestId("kubernetes-mobile-session-list").textContent).toContain("Retained");
  });

  it("shows retained state and main-container requests on both layouts", () => {
    const state = sessionsState();
    const rendered = render(<KubernetesSessionsCard state={state} />);

    expect(screen.getByTestId("kubernetes-sessions-table").textContent).toContain("Retained");
    expect(screen.getByTestId("kubernetes-session-request-summary").textContent).toContain(
      "0 CPU · 512Mi memory",
    );
    expect(screen.getByTestId("kubernetes-session-guidance").textContent).toContain(
      "Stop preserves Kubernetes resources",
    );
    expect(screen.getByTestId("kubernetes-session-guidance").textContent).toContain(
      "Existing claims are not deleted by Kandev",
    );
    const desktopLink = screen.getByTestId("kubernetes-session-task-link");
    expect(desktopLink.getAttribute("href")).toBe("/t/task-retained");
    expect(desktopLink.getAttribute("aria-label")).toBe(
      "Open task task-retained, session session-retained",
    );

    responsive.isMobile = true;
    rendered.rerender(<KubernetesSessionsCard state={state} />);
    const mobile = screen.getByTestId("kubernetes-mobile-session-list");
    expect(within(mobile).getByText("Retained")).toBeTruthy();
    expect(within(mobile).getByTestId("kubernetes-session-request-summary").textContent).toContain(
      "0 CPU · 512Mi memory",
    );
    const mobileLink = within(mobile).getByRole("link");
    expect(mobileLink.getAttribute("href")).toBe("/t/task-retained");
    expect(mobileLink.getAttribute("aria-label")).toBe(
      "Open task task-retained, session session-retained",
    );
    expect(mobileLink.className).toContain("cursor-pointer");
  });

  it("shows Pod phase and main-container state separately on both layouts", () => {
    responsive.isMobile = false;
    const state = sessionsState();
    state.sessions[0] = {
      ...state.sessions[0],
      pod_phase: "Pending",
      container_state: "waiting",
    };

    const rendered = render(<KubernetesSessionsCard state={state} />);
    const desktopStatus = screen.getByTestId("kubernetes-session-status");
    expect(within(desktopStatus).getByText("Pod: Pending")).toBeTruthy();
    expect(within(desktopStatus).getByText("Container: Waiting")).toBeTruthy();

    responsive.isMobile = true;
    rendered.rerender(<KubernetesSessionsCard state={state} />);
    const mobile = screen.getByTestId("kubernetes-mobile-session-list");
    expect(within(mobile).getByText("Pod: Pending")).toBeTruthy();
    expect(within(mobile).getByText("Container: Waiting")).toBeTruthy();
  });
});

type KubernetesSessionsState = ComponentProps<typeof KubernetesSessionsCard>["state"];

function sessionsState(): KubernetesSessionsState {
  return {
    sessions: [
      {
        task_id: "task-retained",
        session_id: "session-retained",
        pod_name: "kandev-retained",
        pod_phase: "Running",
        container_state: "running",
        restarts: 0,
        workspace_kind: "managed_pvc",
        created_at: "2026-08-24T10:00:00Z",
        session_state: "CANCELLED",
        retention_state: "retained",
        main_container_requests: { cpu: "0", memory: "512Mi" },
      },
    ],
    loading: false,
    error: null,
    refresh: vi.fn(async () => []),
  };
}
