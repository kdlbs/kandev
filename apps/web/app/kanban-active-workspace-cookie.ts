import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
} from "@/lib/routing/route-bootstrap";

type CookieReader = {
  get(name: string): { value?: string } | undefined;
};

export function readKanbanActiveWorkspaceCookie(cookieStore: CookieReader | null): string | null {
  if (!cookieStore) return null;
  return (
    cookieStore.get(ACTIVE_WORKSPACE_COOKIE)?.value ??
    cookieStore.get(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)?.value ??
    null
  );
}
