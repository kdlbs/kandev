import { useEffect, useRef } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useArchivedTaskState } from "@/components/task/task-archived-context";
import { useOptionalPortForwardingVisibility } from "@/components/task/port-forwarding-visibility-provider";
import {
  MobileTaskNavigationProvider,
  useMobileTaskNavigationOutlet,
} from "./mobile-task-navigation-provider";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ workspaces: { activeId: "workspace" }, workflows: { activeId: "workflow" } }),
}));
vi.mock("@/components/task/mobile/session-task-switcher-sheet", () => ({
  SessionTaskSwitcherSheet: () => {
    const archived = useArchivedTaskState();
    const ports = useOptionalPortForwardingVisibility();
    return <output>{`${archived.archivedTaskId ?? "none"}:${ports?.enabled ?? false}`}</output>;
  },
}));
afterEach(cleanup);

const portForwarding = {
  enabled: true,
  canToggle: true,
  isUpdating: false,
  dialogOpen: false,
  setDialogOpen: vi.fn(),
  togglePortForwarding: vi.fn(),
};
function Outlet({ taskId }: { taskId: string }) {
  const register = useMobileTaskNavigationOutlet();
  const element = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const outlet = {
      element: element.current!,
      close: vi.fn(),
      navigate: vi.fn(),
      archivedState: { isArchived: true, archivedTaskId: taskId },
      portForwarding,
    };
    register(outlet);
    return () => register(null);
  }, [register, taskId]);
  return <div ref={element} />;
}

it("bridges current task context to the retained sidebar and updates it with the outlet", async () => {
  const host = render(
    <MobileTaskNavigationProvider>
      <Outlet taskId="archived-first" />
    </MobileTaskNavigationProvider>,
  );
  await expect.poll(() => screen.getByRole("status").textContent).toBe("archived-first:true");
  host.rerender(
    <MobileTaskNavigationProvider>
      <Outlet taskId="archived-second" />
    </MobileTaskNavigationProvider>,
  );
  await expect.poll(() => screen.getByRole("status").textContent).toBe("archived-second:true");
});
