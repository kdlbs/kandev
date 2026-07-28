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

// SsoProvider is one external-login button on the pre-auth login screen,
// contributed by an auth-capable plugin. Mirrors apps/backend/internal/auth
// state.go SSOProvider. Present only in the anonymous boot payload.
export type SsoProvider = {
  id: string;
  displayName: string;
  initiateUrl: string;
};

export type AuthSliceState = {
  auth: {
    mode: AuthMode;
    authenticated: boolean;
    user: AuthUser | null;
    // External-login options for the login screen. Optional: GET
    // /api/v1/auth/me omits it, only the anonymous boot payload carries it.
    ssoProviders?: SsoProvider[];
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
