import { useRef } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { sshConnectionSettingsPath } from "@/lib/settings/ssh-connection-settings-routes";
export function useManagerNavigation() {
  const router = useRouter();
  const search = useSearchParams();
  const identities = search.get("tab") === "identities";
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
        mode: "edit",
      }),
    );
  return {
    identities,
    editing: search.get("mode") === "edit" || Boolean(search.get("select_identity")),
    section: ["access", "diagnostics"].includes(search.get("section") ?? "")
      ? search.get("section")!
      : "overview",
    edit: () => router.push(path({ mode: "edit", section: null })),
    cancelEdit: () => router.push(path({ mode: null, section: null, select_identity: null })),
    setSection: (section: string) => router.push(path({ section, mode: null })),
    saved: (id: string, ready: boolean) =>
      router.push(path({ ...list, id, section: ready ? null : "access" })),
    selected: search.get("id"),
    creating: search.get("new") === "1",
    adoptExecutor: search.get("adopt_executor"),
    adoptDigest: search.get("adopt_digest"),
    adoptConnection: search.get("adopt_connection"),
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
          return_connection: null,
        }),
      ),
  };
}
export type ManagerNavigation = ReturnType<typeof useManagerNavigation>;
