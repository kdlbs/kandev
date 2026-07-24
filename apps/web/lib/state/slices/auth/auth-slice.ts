import type { StateCreator } from "zustand";
import type { AuthSlice, AuthSliceState } from "./types";

export const defaultAuthState: AuthSliceState = {
  // IMPORTANT: authenticated defaults to true (with mode "disabled") so that
  // deployments/tests whose boot payload has no auth block at all (older
  // backend, or a test harness that never sets initialState.auth) preserve
  // today's behavior: the app shell renders immediately, no login gate.
  auth: {
    mode: "disabled",
    authenticated: true,
    user: null,
  },
};

type ImmerSet = Parameters<StateCreator<AuthSlice, [["zustand/immer", never]], [], AuthSlice>>[0];

export const createAuthSlice: StateCreator<AuthSlice, [["zustand/immer", never]], [], AuthSlice> = (
  set: ImmerSet,
) => ({
  ...defaultAuthState,
  setAuthState: (state) =>
    set((draft) => {
      draft.auth = state;
    }),
  clearAuthenticated: () =>
    set((draft) => {
      draft.auth.authenticated = false;
      draft.auth.user = null;
    }),
});
