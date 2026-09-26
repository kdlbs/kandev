/**
 * Shared helpers for the auth user-management E2E specs
 * (users-self-actions.spec.ts desktop and mobile-users-self-actions.spec.ts).
 */

/** Reads `user.id` from an API response body, narrowing the unknown shape. */
export function responseUserId(body: unknown): string {
  if (body && typeof body === "object" && "user" in body) {
    const user = body.user;
    if (user && typeof user === "object" && "id" in user) {
      return String(user.id);
    }
  }
  throw new Error("API response is missing user.id");
}
