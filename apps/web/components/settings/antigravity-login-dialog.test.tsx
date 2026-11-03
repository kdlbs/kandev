import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

const {
  startAntigravityLogin,
  getAntigravityLogin,
  cancelAntigravityLogin,
  copyToClipboard,
  responsive,
} = vi.hoisted(() => ({
  startAntigravityLogin: vi.fn(),
  getAntigravityLogin: vi.fn(),
  cancelAntigravityLogin: vi.fn(),
  copyToClipboard: vi.fn(),
  responsive: { isMobile: false },
}));

vi.mock("@/lib/api/domains/antigravity-auth-api", () => ({
  startAntigravityLogin,
  getAntigravityLogin,
  cancelAntigravityLogin,
}));

vi.mock("@/lib/utils/copy-to-clipboard", () => ({ copyToClipboard }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: responsive.isMobile,
    isTablet: false,
    isFinePointer: !responsive.isMobile,
  }),
}));

import { AntigravityLoginDialog } from "./antigravity-login-dialog";

const active = {
  attempt_id: "attempt-1",
  state: "waiting_for_browser" as const,
  expires_at: "2099-01-01T00:00:00Z",
  authorization_url:
    "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb",
};

afterEach(() => {
  vi.clearAllMocks();
  responsive.isMobile = false;
});

describe("AntigravityLoginDialog", () => {
  it("starts, shows the consent URL, copies it, and cancels", async () => {
    getAntigravityLogin.mockRejectedValue(new Error("not found"));
    startAntigravityLogin.mockResolvedValue(active);
    cancelAntigravityLogin.mockResolvedValue({ ...active, state: "cancelled" });
    copyToClipboard.mockResolvedValue(true);
    const onOpenChange = vi.fn();

    render(<AntigravityLoginDialog open onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByRole("button", { name: "Sign in with Google" }));
    await waitFor(() => expect(startAntigravityLogin).toHaveBeenCalled());
    expect((await screen.findByRole("status")).textContent).toContain("Waiting for Google sign-in");

    fireEvent.click(screen.getByRole("button", { name: "Copy link" }));
    expect(copyToClipboard).toHaveBeenCalledWith(active.authorization_url);
    expect(await screen.findByText("Copied")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(cancelAntigravityLogin).toHaveBeenCalledWith("attempt-1"));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("renders one phone action group with copy and cancel", async () => {
    responsive.isMobile = true;
    getAntigravityLogin.mockResolvedValue(active);
    render(<AntigravityLoginDialog open onOpenChange={vi.fn()} />);

    await waitFor(() => expect(getAntigravityLogin).toHaveBeenCalled());
    expect(await screen.findByTestId("antigravity-login-sheet")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Copy link" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Cancel" })).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Sign in with Google" })).toBeNull();
  });

  it("keeps the dialog open and shows an error when cancellation fails", async () => {
    getAntigravityLogin.mockResolvedValue(active);
    cancelAntigravityLogin.mockRejectedValue(new Error("cancel failed"));
    const onOpenChange = vi.fn();
    render(<AntigravityLoginDialog open onOpenChange={onOpenChange} />);

    await waitFor(() => expect(getAntigravityLogin).toHaveBeenCalled());
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
