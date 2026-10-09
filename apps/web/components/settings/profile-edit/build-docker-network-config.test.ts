import { describe, expect, it } from "vitest";
import {
  applyDockerNetworks,
  dockerNetworksInvalidReasonKey,
  serializeAdditionalNetworks,
  type DockerNetworkFormValues,
} from "./build-docker-network-config";

function values(overrides: Partial<DockerNetworkFormValues> = {}): DockerNetworkFormValues {
  return {
    isDocker: true,
    primaryNetwork: "",
    primaryGwPriority: "",
    additionalNetworks: [],
    ...overrides,
  };
}

describe("applyDockerNetworks", () => {
  it("writes no keys for an untouched card", () => {
    const config: Record<string, string> = {};
    applyDockerNetworks(config, values());
    expect(config).toEqual({});
  });

  it("writes nothing for a non-Docker profile", () => {
    const config: Record<string, string> = {};
    applyDockerNetworks(config, values({ isDocker: false, primaryNetwork: "lan" }));
    expect(config).toEqual({});
  });

  it("drops a gateway priority that has no primary network", () => {
    const config: Record<string, string> = {};
    applyDockerNetworks(config, values({ primaryGwPriority: "10" }));
    expect(config).toEqual({});
  });

  it("writes a trimmed primary network and its priority", () => {
    const config: Record<string, string> = {};
    applyDockerNetworks(config, values({ primaryNetwork: " kandev ", primaryGwPriority: " 5 " }));
    expect(config).toEqual({ docker_network: "kandev", docker_network_gw_priority: "5" });
  });

  it("drops empty additional rows and keeps named ones", () => {
    const config: Record<string, string> = {};
    applyDockerNetworks(
      config,
      values({
        additionalNetworks: [
          { name: " ", gwPriority: "3" },
          { name: "lan", gwPriority: "" },
        ],
      }),
    );
    expect(JSON.parse(config.docker_additional_networks)).toEqual([{ name: "lan" }]);
  });
});

describe("serializeAdditionalNetworks", () => {
  it("writes the priority as a number and omits an empty one", () => {
    expect(
      serializeAdditionalNetworks([
        { name: " lan ", gwPriority: " 10 " },
        { name: "db", gwPriority: "" },
      ]),
    ).toEqual([{ name: "lan", gw_priority: 10 }, { name: "db" }]);
  });
});

describe("dockerNetworksInvalidReasonKey", () => {
  const reason = "executors:gatewayPriorityMustBeAWholeNumber";

  it("accepts whole-number priorities, including negative ones", () => {
    expect(
      dockerNetworksInvalidReasonKey(
        values({
          primaryNetwork: "kandev",
          primaryGwPriority: "-1",
          additionalNetworks: [{ name: "lan", gwPriority: "10" }],
        }),
      ),
    ).toBeNull();
  });

  it("refuses a fractional primary priority", () => {
    expect(
      dockerNetworksInvalidReasonKey(
        values({ primaryNetwork: "kandev", primaryGwPriority: "1.5" }),
      ),
    ).toBe(reason);
  });

  it("refuses a fractional additional priority", () => {
    expect(
      dockerNetworksInvalidReasonKey(
        values({ additionalNetworks: [{ name: "lan", gwPriority: "2.5" }] }),
      ),
    ).toBe(reason);
  });

  it("ignores priorities that are not saved", () => {
    expect(
      dockerNetworksInvalidReasonKey(
        values({
          primaryGwPriority: "1.5",
          additionalNetworks: [{ name: "", gwPriority: "2.5" }],
        }),
      ),
    ).toBeNull();
    expect(
      dockerNetworksInvalidReasonKey(
        values({ isDocker: false, primaryNetwork: "x", primaryGwPriority: "1.5" }),
      ),
    ).toBeNull();
  });
});
