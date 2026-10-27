import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useExecutionCommandPreview } from "./use-execution-command-preview";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
const fetchSpy = vi.fn<typeof fetch>();
const composition = {
  agent: { executable: "agent", omitted_arguments: 2, references: [] },
  wrapper: null,
};
let store: ReturnType<typeof useAppStoreApi>;
function ReadStore({ children }: { children: ReactNode }) {
  store = useAppStoreApi();
  return children;
}
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <StateProvider>
        <ReadStore>{children}</ReadStore>
      </StateProvider>
    </QueryClientProvider>
  );
}
function response(value = composition) {
  return new Response(JSON.stringify({ supported: true, composition: value }), {
    headers: { "Content-Type": "application/json" },
  });
}
beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("reads only safe saved defaults with sandbox plugins disabled and refreshes after settings updates", async () => {
  fetchSpy.mockResolvedValue(response());
  const { result } = renderHook(() => useExecutionCommandPreview("profile/a"), { wrapper });
  await waitFor(() => expect(result.current.data).toEqual(composition));
  expect(fetchSpy.mock.calls[0][0]).toBe(
    "http://api.test/api/v1/agent-profiles/profile%2Fa/command-preview",
  );
  expect(fetchSpy.mock.calls[0][1]?.body).toBeUndefined();
  expect(fetchSpy.mock.calls[0][1]?.cache).toBe("no-store");
  const updated = { ...composition, agent: { ...composition.agent, executable: "updated-agent" } };
  fetchSpy.mockResolvedValue(response(updated));
  act(() => store.getState().bumpAgentProfilesVersion());
  await waitFor(() => expect(result.current.data).toEqual(updated));
});

it("hides a deferred response after sign-out and does not read without configuration authority", async () => {
  let finish!: (value: Response) => void;
  fetchSpy.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result } = renderHook(() => useExecutionCommandPreview("profile"), { wrapper });
  await waitFor(() => expect(fetchSpy).toHaveBeenCalledOnce());
  act(() => store.getState().setAuthState({ mode: "enabled", authenticated: false, user: null }));
  await act(async () => finish(response()));
  expect(result.current.data).toBeUndefined();
  expect(fetchSpy).toHaveBeenCalledOnce();
});

it("does not replace a new profile with a late response for the previous profile", async () => {
  let finish!: (value: Response) => void;
  fetchSpy.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  fetchSpy.mockResolvedValue(response());
  const { result, rerender } = renderHook(({ id }) => useExecutionCommandPreview(id), {
    initialProps: { id: "old" },
    wrapper,
  });
  await waitFor(() => expect(fetchSpy).toHaveBeenCalledOnce());
  rerender({ id: "new" });
  await waitFor(() => expect(result.current.data).toEqual(composition));
  await act(async () =>
    finish(response({ ...composition, agent: { ...composition.agent, executable: "old-agent" } })),
  );
  expect(result.current.data).toEqual(composition);
});
