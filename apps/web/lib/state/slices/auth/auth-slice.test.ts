import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createAuthSlice, defaultAuthState } from "./auth-slice";
import type { AuthSlice } from "./types";

function makeStore() {
  return create<AuthSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createAuthSlice as any)(...a) })),
  );
}

describe("auth slice", () => {
  // Production-safety invariant: a boot payload / test harness with no auth
  // block at all must not gate the app shell. See auth-slice.ts for why.
  it("defaults to disabled mode with authenticated true and no user", () => {
    const store = makeStore();
    expect(defaultAuthState.auth.mode).toBe("disabled");
    expect(defaultAuthState.auth.authenticated).toBe(true);
    expect(store.getState().auth.mode).toBe("disabled");
    expect(store.getState().auth.authenticated).toBe(true);
    expect(store.getState().auth.user).toBeNull();
  });

  it("setAuthState replaces the whole snapshot", () => {
    const store = makeStore();
    store.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id: "user-1",
        email: "a@b.com",
        display_name: "A B",
        role: "admin",
        status: "active",
      },
    });
    expect(store.getState().auth.mode).toBe("enabled");
    expect(store.getState().auth.authenticated).toBe(true);
    expect(store.getState().auth.user?.email).toBe("a@b.com");
  });

  it("clearAuthenticated marks the session unauthenticated and drops the user", () => {
    const store = makeStore();
    store.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id: "user-1",
        email: "a@b.com",
        display_name: "A B",
        role: "member",
        status: "active",
      },
    });
    store.getState().clearAuthenticated();
    expect(store.getState().auth.authenticated).toBe(false);
    expect(store.getState().auth.user).toBeNull();
    // mode is left untouched — clearAuthenticated only reflects session expiry.
    expect(store.getState().auth.mode).toBe("enabled");
  });

  it("keeps newer hostname entries and clears them across identities", () => {
    const store = makeStore();
    store
      .getState()
      .setSessionHostname("192.0.2.10", "old.example.test", "2026-08-16T12:00:00.000000001Z");
    store
      .getState()
      .setSessionHostname("192.0.2.10", "new.example.test", "2026-08-16T12:00:00.000000002Z");
    store
      .getState()
      .setSessionHostname("192.0.2.10", "late.example.test", "2026-08-16T12:00:00.000000001Z");
    expect(store.getState().sessionHostnames["192.0.2.10"]).toEqual({
      hostname: "new.example.test",
      resolvedAt: "2026-08-16T12:00:00.000000002Z",
    });
    const epoch = store.getState().sessionHostnamesEpoch;
    store.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id: "user-2",
        email: "user@example.test",
        display_name: "User",
        role: "member",
        status: "active",
      },
    });
    expect(store.getState().sessionHostnames).toEqual({});
    expect(store.getState().sessionHostnamesEpoch).toBe(epoch + 1);
  });
});
