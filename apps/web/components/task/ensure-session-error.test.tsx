import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";

vi.mock("next/link", () => ({
  default: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import {
  describeEnsureError,
  EnsureSessionErrorBanner,
  EnsureSessionErrorEmptyState,
} from "./ensure-session-error";

afterEach(cleanup);

describe("describeEnsureError", () => {
  it("returns null when there is no error", () => {
    expect(describeEnsureError(null)).toBeNull();
  });

  it("detects the missing-agent-profile case and links to workspace settings", () => {
    const info = describeEnsureError(new Error("task has no agent_profile_id configured"), "ws-1");
    expect(info).not.toBeNull();
    expect(info?.isAgentProfileMissing).toBe(true);
    expect(info?.action).toEqual({
      label: "Open workspace settings",
      href: "/settings/workspace/ws-1",
    });
  });

  it("returns a missing-agent-profile descriptor without an action when workspaceId is absent", () => {
    const info = describeEnsureError(new Error("task has no agent_profile_id configured"));
    expect(info?.isAgentProfileMissing).toBe(true);
    expect(info?.action).toBeNull();
  });

  it("falls through to the generic error for unrelated failures", () => {
    const info = describeEnsureError(new Error("websocket disconnected"), "ws-1");
    expect(info?.isAgentProfileMissing).toBe(false);
    expect(info?.title).toBe("Couldn't start a session");
    expect(info?.detail).toContain("websocket disconnected");
    expect(info?.action).toBeNull();
  });

  it("uses a fallback detail when the underlying error has no message", () => {
    const info = describeEnsureError(new Error(""));
    expect(info?.detail).toMatch(/backend rejected/i);
  });
});

describe("EnsureSessionErrorBanner", () => {
  it("renders nothing when there is no error", () => {
    const { container } = render(
      <EnsureSessionErrorBanner error={null} onRetry={() => {}} workspaceId="ws-1" />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders the missing-agent-profile message and a settings link", () => {
    render(
      <EnsureSessionErrorBanner
        error={new Error("task has no agent_profile_id configured")}
        onRetry={() => {}}
        workspaceId="ws-1"
      />,
    );
    expect(screen.getByText("No agent profile configured")).toBeTruthy();
    const link = screen.getByTestId("ensure-session-error-action") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/settings/workspace/ws-1");
  });

  it("invokes onRetry when the retry button is clicked", () => {
    const onRetry = vi.fn();
    render(
      <EnsureSessionErrorBanner error={new Error("websocket disconnected")} onRetry={onRetry} />,
    );
    fireEvent.click(screen.getByTestId("ensure-session-error-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});

describe("EnsureSessionErrorEmptyState", () => {
  it("renders the centered preview-ensure-error layout with a retry", () => {
    const onRetry = vi.fn();
    render(
      <EnsureSessionErrorEmptyState
        error={new Error("boom")}
        onRetry={onRetry}
        workspaceId={null}
      />,
    );
    expect(screen.getByTestId("preview-ensure-error")).toBeTruthy();
    fireEvent.click(screen.getByTestId("ensure-session-error-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
