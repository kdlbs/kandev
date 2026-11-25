import { describe, expect, it } from "vitest";
import type { SSHIdentity, SSHRoute } from "@/lib/types/http-ssh-connections";
import { connectionState, connectionDiscriminator } from "./connection-presentation";
const identities: SSHIdentity[] = [
  {
    id: "key",
    label: "Build key",
    kind: "managed",
    public_key: "public",
    fingerprint: "key-pin",
    generation: "g",
    version: 1,
  },
];
const route: SSHRoute = {
  destination: {
    id: "connection-12345678",
    name: "Build",
    host: "example.invalid",
    port: 22,
    user: "deploy",
    identity_id: "key",
    fingerprint: "pin",
    public_key: "public",
    version: 1,
    revision: 1,
    verified_route: "stamp",
  },
  stamp: "stamp",
};
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.8, 004.9, 004.10
describe("connection presentation", () => {
  it("requires verification for the exact saved route", () => {
    expect(connectionState(route, identities)).toBe("ready");
    expect(connectionState({ ...route, stamp: "new-stamp" }, identities)).toBe("unchecked");
  });
  it("distinguishes incomplete, unavailable and untrusted setup", () => {
    expect(
      connectionState({ ...route, destination: { ...route.destination, host: "" } }, identities),
    ).toBe("incomplete");
    expect(connectionState(route, [])).toBe("unavailable");
    expect(
      connectionState(
        { ...route, destination: { ...route.destination, fingerprint: "" } },
        identities,
      ),
    ).toBe("untrusted");
  });
  it("keeps uncertain installation visible even with a historical successful stamp", () => {
    expect(
      connectionState(
        {
          ...route,
          latest_enrollment: {
            request_id: "attempt",
            connection_id: route.destination.id,
            stamp: "stamp",
            key_fingerprint: "key-pin",
            created_at: "2026-10-26T00:00:00Z",
            state: "outcome_unknown",
          },
        },
        identities,
      ),
    ).toBe("attention");
  });
  it("distinguishes otherwise identical records without merging them", () => {
    expect(connectionDiscriminator(route, [route])).toBe("");
    expect(
      connectionDiscriminator(route, [
        route,
        { ...route, destination: { ...route.destination, id: "other-id" } },
      ]),
    ).toBe(route.destination.id);
    expect(
      connectionDiscriminator(route, [
        route,
        { ...route, destination: { ...route.destination, id: "other-id", host: "other.invalid" } },
      ]),
    ).toBe("");
  });
});

it("gives a failed fresh probe priority over a saved successful stamp", () => {
  expect(connectionState(route, identities, "stamp")).toBe("attention");
  expect(connectionState(route, identities, "older-stamp")).toBe("ready");
});
