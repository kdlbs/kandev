import { useRef } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { sshConnectionSettingsPath } from "@/lib/settings/ssh-connection-settings-routes";
export function useManagerNavigation() {
  const router = useRouter();
  const search = useSearchParams();
  const section = search.get("section");
  const identities = search.get("tab") === "identities";
  const selected = search.get("id");
  const legacyReview =
    !identities && Boolean(selected && search.get("adopt_connection") === selected);
  const path = (changes: Parameters<typeof sshConnectionSettingsPath>[1]) =>
    sshConnectionSettingsPath(search, changes);
  const list = { id: null, new: null, select_identity: null, mode: null, section: null };
  const lastSelected = useRef<string | null>(search.get("id"));
  if (search.get("id")) lastSelected.current = search.get("id");
  const returnIdentity = search.get("return_identity_connection");
  const returnFromIdentity = (identity?: string) =>
    router.push(
      path({
        ...list,
        tab: "connections",
        id: returnIdentity,
        return_identity_connection: null,
        select_identity: identity ?? null,
        mode: !identity && search.get("return_section") ? null : "edit",
        section: !identity ? search.get("return_section") : null,
        return_section: null,
      }),
    );
  return {
    identities,
    editing: search.get("mode") === "edit" || Boolean(search.get("select_identity")),
    section:
      section === "access" || section === "diagnostics" || (!section && legacyReview)
        ? "access"
        : "overview",
    focusSetupChecks: section === "diagnostics",
    edit: () => router.push(path({ mode: "edit", section: null })),
    cancelEdit: () => router.push(path({ mode: null, section: null, select_identity: null })),
    openRemedy: (
      remedy: "edit" | "access" | "identity",
      connectionId: string,
      identityId?: string,
      returnConnection?: string,
    ) => router.push(path(remedyQuery(search, remedy, connectionId, identityId, returnConnection))),
    setSection: (section: string) => router.push(path({ section, mode: null })),
    saved: (id: string, ready: boolean) =>
      router.push(path({ ...list, id, section: ready ? null : "access" })),
    selected,
    creating: search.get("new") === "1",
    returnConnection: search.get("return_connection"),
    select: (id: string) => router.push(path({ ...list, id })),
    back: () =>
      returnIdentity
        ? returnFromIdentity()
        : router.push(path(list), {
            onNavigated: () =>
              requestAnimationFrame(() =>
                (
                  document.getElementById(`ssh-row-${lastSelected.current}`) ??
                  document.getElementById("ssh-create-resource")
                )?.focus(),
              ),
          }),
    create: () => router.push(path({ ...list, new: "1" })),
    showConnections: () =>
      returnIdentity ? returnFromIdentity() : router.push(path({ ...list, tab: "connections" })),
    showIdentities: () => router.push(path({ ...list, tab: "identities" })),
    returnIdentity,
    identityCreated: (id: string) =>
      returnIdentity ? returnFromIdentity(id) : router.push(path({ ...list, id })),
    createIdentity: (id: string) =>
      router.replace(path({ ...list, tab: "connections", id }), {
        onNavigated: () =>
          router.push(
            path({ ...list, tab: "identities", new: "1", return_identity_connection: id }),
          ),
      }),
    createBastion: (id: string) =>
      router.push(path({ ...list, tab: "connections", new: "1", return_connection: id })),
    returnToDestination: () =>
      router.push(
        path({
          ...list,
          tab: "connections",
          id: search.get("return_connection"),
          section: "access",
          return_connection: null,
        }),
      ),
  };
}
export type ManagerNavigation = ReturnType<typeof useManagerNavigation>;

function remedyQuery(
  search: URLSearchParams,
  remedy: "edit" | "access" | "identity",
  connectionId: string,
  identityId?: string,
  returnConnection?: string,
) {
  return {
    new: null,
    select_identity: null,
    tab: remedy === "identity" ? "identities" : "connections",
    id: remedy === "identity" ? (identityId ?? null) : connectionId,
    mode: remedy === "edit" ? "edit" : null,
    section: remedy === "access" ? "access" : null,
    ...(returnConnection ? { return_connection: returnConnection } : {}),
    ...(remedy === "identity"
      ? { return_identity_connection: search.get("id"), return_section: search.get("section") }
      : {}),
  };
}
