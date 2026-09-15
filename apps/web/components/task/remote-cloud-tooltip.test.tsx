import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";

const mocks = vi.hoisted(() => ({
  getKubernetesTaskSession: vi.fn(),
  wsRequest: vi.fn(),
  touch: false,
}));
const STATUS_TRIGGER_ID = "remote-executor-status-trigger";
const EAGER_TASK_ID = "task-eager";
const EAGER_SESSION_ID = "session-eager";

vi.mock("@/lib/api/domains/kubernetes-api", () => ({
  getKubernetesTaskSession: mocks.getKubernetesTaskSession,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.wsRequest }),
}));

beforeEach(() => {
  mocks.getKubernetesTaskSession.mockReset();
  mocks.wsRequest.mockReset();
  mocks.touch = false;
});

afterEach(() => cleanup());

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div data-testid="tooltip-content" {...props}>
      {children}
    </div>
  ),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => mocks.touch,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DrawerTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerHeader: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerTitle: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h2 {...props}>{children}</h2>
  ),
  DrawerDescription: ({ children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) => (
    <p {...props}>{children}</p>
  ),
  DrawerClose: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

function openTooltip() {
  fireEvent.pointerEnter(screen.getByTestId(STATUS_TRIGGER_ID), {
    pointerType: "mouse",
  });
}

function reopenTooltip() {
  fireEvent.pointerLeave(screen.getByTestId(STATUS_TRIGGER_ID), {
    pointerType: "mouse",
  });
  openTooltip();
}

describe("RemoteCloudTooltip executor icon", () => {
  it("shows a container icon for Docker executors", () => {
    render(
      <RemoteCloudTooltip
        taskId="task-1"
        executorType="local_docker"
        fallbackName="Docker"
        status={{ remote_checked_at: new Date().toISOString() }}
      />,
    );

    expect(screen.getByTestId("executor-status-container-icon")).toBeTruthy();
    expect(screen.queryByTestId("executor-status-cloud-icon")).toBeNull();
  });

  it("keeps the cloud icon for Sprites executors", () => {
    render(
      <RemoteCloudTooltip
        taskId="task-1"
        executorType="sprites"
        fallbackName="Sprites.dev"
        status={{ remote_checked_at: new Date().toISOString() }}
      />,
    );

    expect(screen.getByTestId("executor-status-cloud-icon")).toBeTruthy();
  });

  it("uses a keyboard-focusable semantic status trigger", () => {
    render(
      <RemoteCloudTooltip
        taskId="task-trigger"
        executorType="k8s"
        fallbackName="Cluster executor"
        status={{ remote_checked_at: new Date().toISOString() }}
      />,
    );

    const trigger = screen.getByTestId(STATUS_TRIGGER_ID);
    expect(trigger.getAttribute("role")).toBe("img");
    expect(trigger.getAttribute("tabindex")).toBe("0");
    expect(trigger.getAttribute("aria-label")).toContain("Cluster executor");
  });
});

describe("RemoteCloudTooltip relative timestamps", () => {
  it("renders created and last-check times as relative values", () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    const fiveMinutesAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();

    render(
      <RemoteCloudTooltip
        taskId="task-1"
        executorType="sprites"
        fallbackName="Sprites.dev"
        status={{
          remote_name: "kandev-da7d150f-585",
          remote_state: "running",
          remote_created_at: twoHoursAgo,
          remote_checked_at: fiveMinutesAgo,
        }}
      />,
    );

    expect(screen.getByTestId("remote-executor-status-created").textContent).toContain("2h ago");
    expect(screen.getByTestId("remote-executor-status-checked").textContent).toContain("5m ago");
  });
});

describe("RemoteCloudTooltip live status", () => {
  it("eagerly uses exact Kubernetes status and refetches after each reopen", async () => {
    mocks.getKubernetesTaskSession
      .mockResolvedValueOnce({
        task_id: EAGER_TASK_ID,
        session_id: EAGER_SESSION_ID,
        pod_name: "pod-1",
        pod_phase: "Running",
        restarts: 0,
        failure_reason: "Unauthorized",
      })
      .mockResolvedValueOnce({
        task_id: EAGER_TASK_ID,
        session_id: EAGER_SESSION_ID,
        pod_name: "pod-1",
        pod_phase: "Running",
        restarts: 0,
      })
      .mockResolvedValueOnce({
        task_id: EAGER_TASK_ID,
        session_id: EAGER_SESSION_ID,
        pod_name: "pod-1",
        pod_phase: "Running",
        restarts: 0,
      });
    render(
      <RemoteCloudTooltip
        taskId={EAGER_TASK_ID}
        sessionId={EAGER_SESSION_ID}
        executorId="executor-eager"
        executorType="k8s"
      />,
    );

    await waitFor(() =>
      expect(mocks.getKubernetesTaskSession).toHaveBeenCalledWith(
        "executor-eager",
        EAGER_TASK_ID,
        EAGER_SESSION_ID,
      ),
    );
    expect((await screen.findByTestId("remote-executor-status-error")).textContent).toContain(
      "Unauthorized",
    );
    expect(mocks.wsRequest).not.toHaveBeenCalled();

    openTooltip();
    await waitFor(() => expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByTestId("remote-executor-status-error")).toBeNull());
    expect(screen.getByTestId("remote-executor-status-state").textContent).toContain("Running");

    reopenTooltip();
    await waitFor(() => expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(3));
  });

  it("keeps the newest exact Kubernetes response when scope changes", async () => {
    let resolveFirst: ((value: unknown) => void) | undefined;
    mocks.getKubernetesTaskSession
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce({
        task_id: "task-new",
        session_id: "session-new",
        pod_name: "new-pod",
        pod_phase: "Running",
        restarts: 0,
      });
    const { rerender } = render(
      <RemoteCloudTooltip
        taskId="task-old"
        sessionId="session-old"
        executorId="executor-old"
        executorType="k8s"
      />,
    );
    openTooltip();
    await waitFor(() => expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(1));

    rerender(
      <RemoteCloudTooltip
        taskId="task-new"
        sessionId="session-new"
        executorId="executor-new"
        executorType="k8s"
      />,
    );
    await waitFor(() => expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("new-pod")).toBeTruthy();
    resolveFirst?.({
      task_id: "task-1",
      session_id: "session-1",
      pod_name: "stale-pod",
      pod_phase: "Failed",
      restarts: 0,
    });
    await waitFor(() => expect(screen.queryByText("stale-pod")).toBeNull());
    expect(screen.getByText("new-pod")).toBeTruthy();
  });
});

describe("RemoteCloudTooltip shared status presentation", () => {
  it("uses the pending tone for a waiting Kubernetes container", () => {
    render(
      <RemoteCloudTooltip
        taskId="task-waiting"
        executorType="k8s"
        fallbackName="pod-waiting"
        status={{ remote_state: "waiting", remote_checked_at: new Date().toISOString() }}
      />,
    );

    const row = screen.getByTestId("remote-executor-status-state");
    expect(row.querySelector("dd")?.className).toContain("text-amber-600");
  });

  it("shares one eager request across duplicate exact-scope consumers", async () => {
    mocks.getKubernetesTaskSession.mockResolvedValue({
      task_id: "task-duplicate",
      session_id: "session-duplicate",
      pod_name: "pod-1",
      pod_phase: "Running",
      restarts: 0,
    });

    render(
      <>
        <RemoteCloudTooltip
          taskId="task-duplicate"
          sessionId="session-duplicate"
          executorId="executor-duplicate"
          executorType="k8s"
        />
        <RemoteCloudTooltip
          taskId="task-duplicate"
          sessionId="session-duplicate"
          executorId="executor-duplicate"
          executorType="k8s"
        />
      </>,
    );

    await waitFor(() => expect(screen.getAllByText("pod-1")).toHaveLength(2));
    expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(1);
  });

  it("renders a structured Kubernetes task status summary", async () => {
    mocks.getKubernetesTaskSession.mockResolvedValue({
      task_id: "task-structured",
      session_id: "session-structured",
      pod_name: "pod-structured",
      pod_phase: "Running",
      container_state: "running",
      restarts: 4,
      workspace_kind: "managed_pvc",
      created_at: "2026-09-01T09:00:00Z",
    });
    render(
      <RemoteCloudTooltip
        taskId="task-structured"
        sessionId="session-structured"
        executorId="executor-structured"
        executorType="k8s"
      />,
    );

    const summary = await screen.findByTestId("remote-executor-status-summary");
    expect(summary.className).toContain("w-full");
    expect(screen.getByTestId("remote-executor-status-identity").textContent).toBe(
      "pod-structured",
    );
    expect(screen.getByTestId("remote-executor-status-state").textContent).toContain("running");
    expect(screen.getByTestId("remote-executor-status-restarts").textContent).toContain("4");
    expect(screen.getByTestId("remote-executor-status-workspace").textContent).toContain(
      "managed_pvc",
    );
  });
});

describe("RemoteCloudTooltip compatibility status", () => {
  it("keeps WebSocket status for other remote executors", async () => {
    mocks.wsRequest.mockResolvedValue({
      remote_name: "sprite-1",
      remote_state: "running",
      remote_checked_at: new Date().toISOString(),
    });
    render(
      <RemoteCloudTooltip
        taskId="task-compatible"
        sessionId="session-compatible"
        executorId="executor-compatible"
        executorType="sprites"
      />,
    );

    openTooltip();
    await waitFor(() => expect(mocks.wsRequest).toHaveBeenCalledTimes(1));
    expect(mocks.getKubernetesTaskSession).not.toHaveBeenCalled();
    reopenTooltip();
    await waitFor(() => expect(mocks.wsRequest).toHaveBeenCalledTimes(2));
  });
});

describe("RemoteCloudTooltip coarse-pointer disclosure", () => {
  it("opens the shared summary from a 44px task-safe Drawer trigger", () => {
    mocks.touch = true;
    const onParentClick = vi.fn();
    render(
      <div onClick={onParentClick}>
        <RemoteCloudTooltip
          taskId="task-touch"
          executorType="k8s"
          fallbackName="pod-touch"
          status={{
            remote_name: "pod-touch",
            remote_state: "running",
            remote_checked_at: new Date().toISOString(),
            remote_restarts: 2,
            remote_workspace_kind: "empty_dir",
          }}
        />
      </div>,
    );

    const trigger = screen.getByTestId(STATUS_TRIGGER_ID);
    expect(trigger.getAttribute("role")).toBe("button");
    expect(trigger.getAttribute("aria-haspopup")).toBe("dialog");
    expect(trigger.className).toContain("after:size-11");
    fireEvent.click(trigger);
    expect(onParentClick).not.toHaveBeenCalled();
    expect(screen.getByTestId("remote-executor-status-drawer")).toBeTruthy();
    expect(screen.getByTestId("remote-executor-status-workspace").textContent).toContain(
      "empty_dir",
    );
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onParentClick).not.toHaveBeenCalled();
  });

  it("does not refresh repeatedly for auto-repeated activation keys", async () => {
    mocks.touch = true;
    mocks.getKubernetesTaskSession.mockResolvedValue({
      task_id: "task-touch-repeat",
      session_id: "session-touch-repeat",
      pod_name: "pod-touch",
      pod_phase: "Running",
      restarts: 0,
    });
    render(
      <RemoteCloudTooltip
        taskId="task-touch-repeat"
        sessionId="session-touch-repeat"
        executorId="executor-touch-repeat"
        executorType="k8s"
      />,
    );

    await waitFor(() => expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(1));
    const trigger = screen.getByTestId(STATUS_TRIGGER_ID);
    fireEvent.keyDown(trigger, { key: "Enter", repeat: true });
    expect(mocks.getKubernetesTaskSession).toHaveBeenCalledTimes(1);
  });
});
