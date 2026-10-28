import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api/domains/ssh-connections-api";
import type { SSHIdentity } from "@/lib/types/http-ssh-connections";
import { ApiError } from "@/lib/api/client";
import { useSSHConnections } from "./use-ssh-connections";

vi.mock("@/lib/api/domains/ssh-connections-api");
afterEach(cleanup);
const identity: SSHIdentity = {
  id: "managed-key",
  label: "Managed key",
  kind: "managed",
  public_key: "ssh-ed25519 public",
  fingerprint: "SHA256:key",
  generation: "generation",
  version: 1,
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.listSSHIdentities).mockResolvedValue([]);
  vi.mocked(api.listManagedSSHConnections).mockResolvedValue([]);
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.3
it("admits one same-tick mutation and keeps busy until its refresh completes", async () => {
  const mutation = deferred<SSHIdentity>();
  const refresh = deferred<SSHIdentity[]>();
  vi.mocked(api.createSSHIdentity).mockReturnValue(mutation.promise);
  const { result } = renderHook(useSSHConnections);
  await waitFor(() => expect(result.current.loaded).toBe(true));
  vi.mocked(api.listSSHIdentities).mockReturnValue(refresh.promise);
  let first!: Promise<SSHIdentity>;
  let duplicate!: Promise<unknown>;
  act(() => {
    first = result.current.createIdentity({ request_id: "first", label: "key", kind: "generate" });
    duplicate = result.current
      .createIdentity({ request_id: "duplicate", label: "key", kind: "generate" })
      .then(
        () => "accepted",
        () => "rejected",
      );
  });
  expect(api.createSSHIdentity).toHaveBeenCalledTimes(1);
  expect(await duplicate).toBe("rejected");
  expect(result.current.busy).toBe(true);
  await act(async () => {
    mutation.resolve(identity);
  });
  expect(result.current.busy).toBe(true);
  await act(async () => {
    refresh.resolve([identity]);
    await first;
  });
  expect(result.current.busy).toBe(false);
  expect(result.current.identities).toEqual([identity]);
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.3
it("does not replace a mutation refresh with an older initial response", async () => {
  const initial = deferred<SSHIdentity[]>();
  vi.mocked(api.listSSHIdentities)
    .mockReturnValueOnce(initial.promise)
    .mockResolvedValue([identity]);
  vi.mocked(api.createSSHIdentity).mockResolvedValue(identity);
  const { result } = renderHook(useSSHConnections);
  await act(async () => {
    await result.current.createIdentity({ request_id: "new", label: "key", kind: "generate" });
  });
  expect(result.current.identities).toEqual([identity]);
  await act(async () => {
    initial.resolve([]);
  });
  expect(result.current.identities).toEqual([identity]);
});

it("releases mutation admission after a failure so the user can retry", async () => {
  vi.mocked(api.createSSHIdentity)
    .mockRejectedValueOnce(new Error("private diagnostic"))
    .mockResolvedValueOnce(identity);
  const { result } = renderHook(useSSHConnections);
  await waitFor(() => expect(result.current.loaded).toBe(true));
  const request = { request_id: "retry", label: "key", kind: "generate" as const };
  await act(async () => {
    await expect(result.current.createIdentity(request)).rejects.toThrow();
  });
  expect(result.current.busy).toBe(false);
  expect(result.current.error).toBe("ssh_operation_failed");
  await act(async () => {
    await result.current.createIdentity(request);
  });
  expect(result.current.error).toBeNull();
  expect(result.current.busy).toBe(false);
  expect(api.createSSHIdentity).toHaveBeenCalledTimes(2);
});

it("does not show an obsolete initial-load error after a newer successful refresh", async () => {
  let reject!: (error: Error) => void;
  const initial = new Promise<SSHIdentity[]>((_resolve, fail) => {
    reject = fail;
  });
  vi.mocked(api.listSSHIdentities).mockReturnValueOnce(initial).mockResolvedValue([identity]);
  vi.mocked(api.createSSHIdentity).mockResolvedValue(identity);
  const { result } = renderHook(useSSHConnections);
  await act(async () => {
    await result.current.createIdentity({ request_id: "new", label: "key", kind: "generate" });
  });
  await act(async () => {
    reject(new Error("old network failure"));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.identities).toEqual([identity]);
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.6
it("refreshes dependencies after another administrator binds the identity before deletion", async () => {
  vi.mocked(api.listSSHIdentities).mockResolvedValue([identity]);
  const { result } = renderHook(useSSHConnections);
  await waitFor(() => expect(result.current.loaded).toBe(true));
  const current = {
    ...identity,
    dependencies: [{ kind: "connection" as const, id: "new-route", label: "New route" }],
  };
  vi.mocked(api.listSSHIdentities).mockResolvedValue([current]);
  vi.mocked(api.deleteSSHIdentity).mockRejectedValue(
    new ApiError("Conflict", 409, { error: "ssh_resource_in_use" }),
  );
  await act(async () => {
    await expect(result.current.deleteIdentity(identity)).rejects.toThrow();
  });
  expect(result.current.identities).toEqual([current]);
  expect(result.current.error).toBe("ssh_resource_in_use");
  expect(result.current.busy).toBe(false);
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.10, 004.3
it("reconciles a failed test instead of retaining the old ready snapshot", async () => {
  const route = {
    destination: { id: "route", verified_route: "stamp" },
    stamp: "stamp",
  } as import("@/lib/types/http-ssh-connections").SSHRoute;
  vi.mocked(api.listManagedSSHConnections).mockResolvedValue([route]);
  const { result } = renderHook(useSSHConnections);
  await waitFor(() => expect(result.current.loaded).toBe(true));
  vi.mocked(api.listManagedSSHConnections).mockResolvedValue([
    { ...route, destination: { ...route.destination, verified_route: "" } },
  ]);
  vi.mocked(api.testManagedSSH).mockRejectedValue(
    new ApiError("Rejected", 422, { error: "ssh_authentication_rejected" }),
  );
  await act(async () => {
    await expect(result.current.test(route)).rejects.toThrow();
  });
  expect(result.current.routes[0].destination.verified_route).toBe("");
  expect(result.current.error).toBe("ssh_authentication_rejected");
  expect(result.current.busy).toBe(false);
});

it("keeps failed probe evidence scoped to its route stamp until a successful retry", async () => {
  const route = {
    destination: { id: "route", verified_route: "stamp" },
    stamp: "stamp",
  } as import("@/lib/types/http-ssh-connections").SSHRoute;
  vi.mocked(api.listManagedSSHConnections).mockResolvedValue([route]);
  const { result } = renderHook(useSSHConnections);
  await waitFor(() => expect(result.current.loaded).toBe(true));
  vi.mocked(api.listManagedSSHConnections).mockRejectedValue(new Error("offline"));
  vi.mocked(api.testManagedSSH).mockRejectedValue(
    new ApiError("Offline", 503, { error: "ssh_network_unreachable" }),
  );
  await act(async () => {
    await expect(result.current.test(route)).rejects.toThrow();
  });
  expect(
    (result.current as unknown as { failedTests: Record<string, string> }).failedTests,
  ).toEqual({ route: "stamp" });
  vi.mocked(api.listManagedSSHConnections).mockResolvedValue([route]);
  vi.mocked(api.testManagedSSH).mockResolvedValue(route);
  await act(async () => {
    await result.current.test(route);
  });
  expect(
    (result.current as unknown as { failedTests: Record<string, string> }).failedTests,
  ).toEqual({});
});
