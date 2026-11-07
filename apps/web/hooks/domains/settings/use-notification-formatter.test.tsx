import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useNotificationFormatter } from "./use-notification-formatter";

const api = vi.hoisted(() => ({ get: vi.fn(), select: vi.fn() }));
vi.mock("@/lib/api/domains/notification-formatters-api", () => ({
  getNotificationFormatter: api.get,
  selectNotificationFormatter: api.select,
  previewNotificationFormatter: vi.fn(),
}));
const binding = {
  provider_id: "mail",
  provider_type: "apprise",
  plugin_id: "formatter",
  installation_id: "installation",
  formatter_key: "text",
  configuration_revision: "snapshot",
  digest: "digest",
  revision: "original",
};
const settings = {
  binding,
  events: [],
  formatters: [
    { plugin_id: "formatter", installation_id: "installation", key: "text", version: "1.0.0" },
  ],
};
const cleared = {
  ...binding,
  plugin_id: "",
  installation_id: "",
  formatter_key: "",
  configuration_revision: "",
  digest: "",
  revision: "cleared",
};

beforeEach(() => {
  vi.resetAllMocks();
  api.get.mockResolvedValue(settings);
});

it("does not publish a delayed read over a successful default selection", async () => {
  const { result } = renderHook(() => useNotificationFormatter("mail"));
  await waitFor(() => expect(result.current.loading).toBe(false));
  let finish!: (value: typeof settings) => void;
  api.get.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  let reading!: Promise<unknown>;
  act(() => {
    reading = result.current.readActiveBinding().catch(() => null);
  });
  act(() => result.current.select(""));
  api.select.mockResolvedValueOnce({ binding: cleared });
  await act(async () => {
    await result.current.clear();
  });
  await act(async () => {
    finish(settings);
    await reading;
  });
  expect(result.current.settings?.binding?.revision).toBe("cleared");
  expect(result.current.selected).toBe("");
});

it("reconciles a lost clear response before considering another write", async () => {
  const { result } = renderHook(() => useNotificationFormatter("mail"));
  await waitFor(() => expect(result.current.loading).toBe(false));
  act(() => result.current.select(""));
  api.select.mockRejectedValue(new Error("lost response"));
  api.get.mockRejectedValue(new Error("readback unavailable"));
  await act(async () => {
    await expect(result.current.clear()).rejects.toThrow();
  });
  await act(async () => {
    await expect(result.current.clear()).rejects.toThrow();
  });
  expect(api.select).toHaveBeenCalledTimes(1);
  api.get.mockResolvedValue({ ...settings, binding: cleared });
  await act(async () => {
    await result.current.clear();
  });
  expect(api.select).toHaveBeenCalledTimes(1);
  expect(result.current.settings?.binding?.revision).toBe("cleared");
});

it("settles loading when authoritative readback supersedes a reload", async () => {
  const { result } = renderHook(() => useNotificationFormatter("mail"));
  await waitFor(() => expect(result.current.loading).toBe(false));
  let finish!: (value: typeof settings) => void;
  api.get.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  let reloading!: Promise<void>;
  act(() => {
    reloading = result.current.reload();
  });
  expect(result.current.loading).toBe(true);
  api.get.mockResolvedValueOnce({ ...settings, binding: cleared });
  await act(async () => {
    await result.current.readActiveBinding();
  });
  await act(async () => {
    finish(settings);
    await reloading;
  });
  expect(result.current.settings?.binding?.revision).toBe("cleared");
  expect(result.current.loading).toBe(false);
});

it("does not publish an old activation acknowledgement over a newer clear", async () => {
  const { result } = renderHook(() => useNotificationFormatter("mail"));
  await waitFor(() => expect(result.current.loading).toBe(false));
  const activated = { ...binding, configuration_revision: "new-snapshot", revision: "activated" };
  let finish!: (value: { binding: typeof binding }) => void;
  api.select.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  let activating!: Promise<unknown>;
  act(() => {
    activating = result.current
      .activate({ revision: "new-snapshot", digest: "digest" }, "original")
      .catch(() => null);
  });
  act(() => result.current.select(""));
  api.select.mockRejectedValueOnce(new Error("CAS conflict"));
  api.get.mockResolvedValueOnce({ ...settings, binding: activated });
  await act(async () => {
    await expect(result.current.clear()).rejects.toThrow();
  });
  api.select.mockResolvedValueOnce({ binding: cleared });
  await act(async () => {
    await result.current.clear();
  });
  api.get.mockResolvedValueOnce({ ...settings, binding: cleared });
  await act(async () => {
    finish({ binding: activated });
    await activating;
  });
  expect(result.current.settings?.binding?.revision).toBe("cleared");
  expect(result.current.selected).toBe("");
});
