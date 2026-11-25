import type { ManagedSSHConnection, SSHIdentity, SSHRoute } from "@/lib/types/http-ssh-connections";

export type ConnectionState =
  | "incomplete"
  | "unavailable"
  | "untrusted"
  | "unchecked"
  | "ready"
  | "attention"
  | "imported";
export function connectionState(
  route: SSHRoute,
  identities: SSHIdentity[],
  failedStamp?: string,
): ConnectionState {
  const c = route.destination;
  if (requiresRouteRepair(route)) return "attention";
  if (!c.host || !c.user || !c.identity_id) return "incomplete";
  if (!identities.some((identity) => identity.id === c.identity_id)) return "unavailable";
  if (!c.fingerprint || !hasBastionTrustEvidence(route.bastion, c.imported_pending))
    return "untrusted";
  if (failedStamp === route.stamp) return "attention";
  const enrollment = route.latest_enrollment;
  if (
    enrollment?.stamp === route.stamp &&
    ["started", "installed_unverified", "outcome_unknown"].includes(enrollment.state)
  )
    return "attention";
  if (c.verified_route === route.stamp) return "ready";
  return c.imported_pending ? "imported" : "unchecked";
}
export function canVerifyOnFirstUse(route: SSHRoute): boolean {
  const { destination, bastion } = route;
  return Boolean(
    destination.imported_pending &&
    destination.fingerprint &&
    !requiresRouteRepair(route) &&
    hasBastionTrustEvidence(bastion, true),
  );
}

function requiresRouteRepair({ destination, bastion }: SSHRoute): boolean {
  return Boolean(destination.repair_reason || bastion?.repair_reason);
}

function hasBastionTrustEvidence(bastion: SSHRoute["bastion"], imported?: boolean): boolean {
  return !bastion || Boolean(bastion.fingerprint || (imported && bastion.known_hosts_pending));
}

export function connectionEndpoint({ host, user, port, alias }: ManagedSSHConnection): string {
  if (!host) return alias ?? "";
  if (!user) return host;
  return port ? `${user}@${host}:${port}` : `${user}@${host}`;
}

export function connectionDiscriminator(route: SSHRoute, routes: SSHRoute[]): string {
  const c = route.destination;
  const duplicate = routes.some(
    ({ destination: other }) =>
      other.id !== c.id &&
      other.name === c.name &&
      other.host === c.host &&
      other.port === c.port &&
      other.user === c.user &&
      other.identity_id === c.identity_id &&
      other.bastion_id === c.bastion_id,
  );
  return duplicate ? c.id : "";
}
