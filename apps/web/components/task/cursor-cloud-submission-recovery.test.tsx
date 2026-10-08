import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CursorCloudSubmissionRecovery } from "./cursor-cloud-submission-recovery";

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <section role="dialog">{children}</section>
  ),
  DialogDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DrawerContent: ({ children }: { children: React.ReactNode }) => (
    <section role="dialog" data-slot="drawer-content">
      {children}
    </section>
  ),
  DrawerDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DrawerHeader: ({ children }: { children: React.ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

afterEach(cleanup);

const resolution = {
  operationId: "operation-1",
  state: "unknown",
  candidates: [
    { runId: "run-1", status: "RUNNING", createdAt: "2026-09-25T09:00:00Z" },
    { runId: "run-2", status: "COMPLETED", createdAt: "2026-09-25T09:05:00Z" },
  ],
};

describe("Cursor Cloud submission recovery", () => {
  it("requires the duplicate-work acknowledgment before retry and can bind a candidate", () => {
    const onBind = vi.fn();
    const onRetry = vi.fn();
    render(
      <CursorCloudSubmissionRecovery
        state="unknown"
        resolution={resolution}
        isMobile={false}
        agentUrl="https://cursor.com/agents/agent-1"
        busy={false}
        onBind={onBind}
        onRetry={onRetry}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain("may have received your message");
    fireEvent.click(screen.getByRole("button", { name: "Resolve submission" }));
    fireEvent.click(screen.getByRole("radio", { name: /run-2/ }));
    fireEvent.click(screen.getByRole("button", { name: "Bind selected run" }));
    expect(onBind).toHaveBeenCalledWith("run-2");

    const retry = screen.getByRole("button", { name: "Retry this message" }) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox"));
    expect(retry.disabled).toBe(false);
    fireEvent.click(retry);
    expect(onRetry).toHaveBeenCalledWith("operation-1");
  });

  it("uses a full-height phone resolution drawer and disables actions while loading", () => {
    render(
      <CursorCloudSubmissionRecovery
        state="loading"
        resolution={null}
        isMobile
        agentUrl={null}
        busy
        onBind={vi.fn()}
        onRetry={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );
    expect(screen.getByRole("status").textContent).toMatch(/checking/i);
    expect(screen.queryByRole("button", { name: "Resolve submission" })).toBeNull();
  });
});
