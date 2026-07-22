import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  getMRSubscription: vi.fn(),
  setMRSubscription: vi.fn(),
  getIssueSubscription: vi.fn(),
  setIssueSubscription: vi.fn(),
}));
const toast = vi.hoisted(() => vi.fn());
const toastAPI = vi.hoisted(() => ({ toast }));
vi.mock("@/lib/api/domains/gitlab-api", () => api);
vi.mock("@/components/toast-provider", () => ({ useToast: () => toastAPI }));
import { SubscriptionToggle, subscriptionActionLabel } from "./subscription-toggle";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("subscriptionActionLabel", () => {
  it("describes the next upstream notification action", () => {
    expect(subscriptionActionLabel(false)).toBe("Subscribe to GitLab notifications");
    expect(subscriptionActionLabel(true)).toBe("Unsubscribe from GitLab notifications");
  });

  it("ignores a delayed subscription read after the MR identity changes", async () => {
    const first = deferred<{ subscribed: boolean }>();
    const second = deferred<{ subscribed: boolean }>();
    api.getMRSubscription.mockImplementation(({ project }: { project: string }) =>
      project === "group/a" ? first.promise : second.promise,
    );
    const shared = { workspaceId: "ws", host: "https://gitlab.example", iid: 1 };
    const { rerender } = render(
      createElement(SubscriptionToggle, { ...shared, project: "group/a" }),
    );
    rerender(createElement(SubscriptionToggle, { ...shared, project: "group/b" }));

    act(() => second.resolve({ subscribed: true }));
    await screen.findByRole("button", { name: "Unsubscribe from GitLab notifications" });
    act(() => first.resolve({ subscribed: false }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Subscribe to GitLab notifications" }),
      ).toBeNull(),
    );
  });

  it("uses the upstream issue subscription endpoints for an issue row", async () => {
    api.getIssueSubscription.mockResolvedValue({ subscribed: false });
    api.setIssueSubscription.mockResolvedValue({ subscribed: true });
    render(
      createElement(SubscriptionToggle, {
        kind: "issue",
        workspaceId: "ws",
        host: "https://gitlab.example",
        project: "group/project",
        iid: 7,
      }),
    );

    const button = await screen.findByRole("button", {
      name: "Subscribe to GitLab notifications",
    });
    button.click();
    await screen.findByRole("button", { name: "Unsubscribe from GitLab notifications" });
    expect(api.setIssueSubscription).toHaveBeenCalledWith({
      workspaceId: "ws",
      host: "https://gitlab.example",
      project: "group/project",
      iid: 7,
      subscribed: true,
    });
    expect(api.getMRSubscription).not.toHaveBeenCalled();
  });
});
