import type { StateCreator } from "zustand";
import type { AuthSlice, AuthSliceState } from "./types";

import type { SessionHostnameEntry } from "./types";
export const defaultAuthState: AuthSliceState = {
  // IMPORTANT: authenticated defaults to true (with mode "disabled") so that
  // deployments/tests whose boot payload has no auth block at all (older
  // backend, or a test harness that never sets initialState.auth) preserve
  // today's behavior: the app shell renders immediately, no login gate.
  auth: {
    mode: "disabled",
    authenticated: true,
    user: null,
    ssoProviders: [],
  },
  sessionHostnames: {},
  sessionHostnamesEpoch: 0,
};

type ImmerSet = Parameters<StateCreator<AuthSlice, [["zustand/immer", never]], [], AuthSlice>>[0];

// The auth slice only needs `set` (no cross-slice reads), so it is typed as a
// plain single-arg creator rather than the full 3-arg StateCreator. `set`
// keeps the immer-enabled recipe typing via ImmerSet.
function incomingHostnameWins(
  current: SessionHostnameEntry,
  incoming: SessionHostnameEntry,
): boolean {
  if (current.resolvedAt !== incoming.resolvedAt) {
    if (current.resolvedAt === null) return incoming.resolvedAt !== null;
    if (incoming.resolvedAt === null) return false;
    return incoming.resolvedAt > current.resolvedAt;
  }
  return current.hostname === "" && incoming.hostname !== "";
}

export const createAuthSlice = (set: ImmerSet): AuthSlice => ({
  ...defaultAuthState,
  setAuthState: (state) =>
    set((draft) => {
      if (draft.auth.user?.id !== state.user?.id) {
        draft.sessionHostnames = {};
        draft.sessionHostnamesEpoch += 1;
      }
      draft.auth = state;
    }),
  clearAuthenticated: () =>
    set((draft) => {
      draft.auth.authenticated = false;
      draft.auth.user = null;
      draft.sessionHostnames = {};
      draft.sessionHostnamesEpoch += 1;
    }),
  setSessionHostname: (ip, hostname, resolvedAt) =>
    set((draft) => {
      const incoming = { hostname, resolvedAt };
      const current = draft.sessionHostnames[ip];
      if (!current || incomingHostnameWins(current, incoming)) {
        draft.sessionHostnames[ip] = incoming;
      }
    }),
});
