import type { SSHIdentity, SSHRoute } from "@/lib/types/http-ssh-connections";

export type ConnectionState =
  | "incomplete"
  | "unavailable"
  | "untrusted"
  | "unchecked"
  | "ready"
  | "attention";
export function connectionState(
  route: SSHRoute,
  identities: SSHIdentity[],
  failedStamp?: string,
): ConnectionState {
  const c = route.destination;
  if (!c.host || !c.user || !c.identity_id) return "incomplete";
  if (!identities.some((identity) => identity.id === c.identity_id)) return "unavailable";
  if (!c.fingerprint || (route.bastion && !route.bastion.fingerprint)) return "untrusted";
  if (failedStamp === route.stamp) return "attention";
  const enrollment = route.latest_enrollment;
  if (
    enrollment?.stamp === route.stamp &&
    ["started", "installed_unverified", "outcome_unknown"].includes(enrollment.state)
  )
    return "attention";
  return c.verified_route === route.stamp ? "ready" : "unchecked";
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
