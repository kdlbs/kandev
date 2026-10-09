import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { getRepositoryCheckoutCapabilities } from "@/lib/api/domains/repository-checkout-api";
import { useRepositoryCheckoutCapabilities } from "./use-repository-checkout-capabilities";
import type { RepositoryCheckoutCapabilities } from "@/lib/types/repository-checkout-options";

vi.mock("@/lib/api/domains/repository-checkout-api", () => ({
  getRepositoryCheckoutCapabilities: vi.fn(),
}));

describe("repository checkout capability changes", () => {
  it("discards the previous executor response after switching profiles", async () => {
    let resolveOld!: (value: RepositoryCheckoutCapabilities) => void;
    const request = vi.mocked(getRepositoryCheckoutCapabilities);
    request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
    );
    request.mockResolvedValueOnce({
      on_demand: false,
      sparse: false,
      reason: "preparation_unsupported",
    });
    const { result, rerender } = renderHook(
      ({ profile }) =>
        useRepositoryCheckoutCapabilities(
          "workspace",
          "https://github.com/acme/repo",
          "github",
          profile,
          { enabled: true },
        ),
      { initialProps: { profile: "worktree" } },
    );
    rerender({ profile: "local" });
    await waitFor(() =>
      expect(result.current?.capabilities?.reason).toBe("preparation_unsupported"),
    );
    await act(async () => resolveOld({ on_demand: true, sparse: true }));
    expect(result.current?.capabilities?.on_demand).toBe(false);
    expect(request.mock.calls[0][4].aborted).toBe(true);
  });
});

it("retries a failed capability request without changing repository identity", async () => {
  const request = vi.mocked(getRepositoryCheckoutCapabilities);
  request.mockRejectedValueOnce(new Error("offline"));
  request.mockResolvedValueOnce({ on_demand: true, sparse: true });
  const { result, rerender } = renderHook(
    ({ attempt }) =>
      useRepositoryCheckoutCapabilities(
        "workspace",
        "https://github.com/acme/repo",
        "github",
        "worktree",
        { enabled: true, attempt },
      ),
    { initialProps: { attempt: 0 } },
  );
  await waitFor(() => expect(result.current?.failed).toBe(true));
  rerender({ attempt: 1 });
  await waitFor(() => expect(result.current?.capabilities?.sparse).toBe(true));
});
