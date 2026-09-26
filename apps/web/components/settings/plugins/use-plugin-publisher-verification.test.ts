import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PluginRecord } from "@/lib/types/plugins";
import { usePluginPublisherVerification } from "./use-plugin-publisher-verification";

afterEach(() => vi.restoreAllMocks());

function plugin(id: string, version: string, installationID: string): PluginRecord {
  return {
    id,
    api_version: 1,
    version,
    display_name: id,
    description: "",
    author: "declared",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: `/plugins/${id}/${version}`,
    installation_id: installationID,
    signed: false,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    publisher_identity: { status: "unverified" },
  };
}

describe("usePluginPublisherVerification", () => {
  it("clears transient state and ignores a completion after navigation", async () => {
    let resolveRequest!: (applied: boolean) => void;
    const request = new Promise<boolean>((resolve) => {
      resolveRequest = resolve;
    });
    const verifyPublisher = vi.fn(() => request);
    const first = plugin("one", "1.0.0", "install-one");
    const second = plugin("two", "1.0.0", "install-two");
    const hook = renderHook(
      ({ current }: { current: PluginRecord | null }) =>
        usePluginPublisherVerification(current, verifyPublisher),
      { initialProps: { current: first } },
    );

    await act(async () => {
      void hook.result.current.verify();
    });
    expect(hook.result.current.busy).toBe(true);

    hook.rerender({ current: second });
    expect(hook.result.current.busy).toBe(false);
    expect(hook.result.current.error).toBeUndefined();
    expect(hook.result.current.success).toBe(false);

    await act(async () => {
      resolveRequest(true);
      await request;
    });
    expect(hook.result.current.busy).toBe(false);
    expect(hook.result.current.success).toBe(false);
  });
});
