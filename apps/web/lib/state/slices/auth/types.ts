// Auth snapshot state. Mirrors apps/backend/internal/auth/state.go StatePayload
// (the shape shared by GET /api/v1/auth/me and the boot payload's
// initialState.auth). Field names stay snake_case where the wire payload uses
// snake_case so hydration from the boot payload/API response is a straight
// pass-through with no remapping.

export type AuthUser = {
  id: string;
  email: string;
  display_name: string;
  role: string;
  status: string;
  created_at?: string;
  updated_at?: string;
};

export type AuthMode = "disabled" | "setup" | "enabled";

export type AuthSliceState = {
  auth: {
    mode: AuthMode;
    authenticated: boolean;
    user: AuthUser | null;
  };
};

export type AuthSliceActions = {
  /** Replace the whole auth snapshot, e.g. after GET /api/v1/auth/me or a
   * successful login/setup/logout response. */
  setAuthState: (state: AuthSliceState["auth"]) => void;
  /** Mark the session unauthenticated without touching mode.
   * Used by the 401 handler when a session expires mid-use. */
  clearAuthenticated: () => void;
};

export type AuthSlice = AuthSliceState & AuthSliceActions;
