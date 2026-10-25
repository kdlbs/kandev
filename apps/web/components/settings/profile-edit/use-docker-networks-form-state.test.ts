import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useDockerNetworksFormState } from "./use-docker-networks-form-state";

const PRIMARY = "lab-bridge";
const MACVLAN = "lan-macvlan";

describe("useDockerNetworksFormState", () => {
  it("starts from the saved profile configuration", () => {
    const { result } = renderHook(() =>
      useDockerNetworksFormState({
        docker_network: PRIMARY,
        docker_network_gw_priority: "0",
        docker_additional_networks: `[{"name":"${MACVLAN}","gw_priority":-10}]`,
      }),
    );

    expect(result.current.primaryNetwork).toBe(PRIMARY);
    expect(result.current.primaryGwPriority).toBe("0");
    expect(result.current.additionalNetworks).toEqual([{ name: MACVLAN, gwPriority: "-10" }]);
  });

  it("starts empty for a profile that configures no network", () => {
    const { result } = renderHook(() => useDockerNetworksFormState(undefined));

    expect(result.current.primaryNetwork).toBe("");
    expect(result.current.primaryGwPriority).toBe("");
    expect(result.current.additionalNetworks).toEqual([]);
  });

  // A stored value that is not a list must not blank the whole editor or throw
  // during render; the operator still needs the rest of the profile.
  it("treats an unreadable additional list as empty", () => {
    const { result } = renderHook(() =>
      useDockerNetworksFormState({ docker_additional_networks: "{not json" }),
    );

    expect(result.current.additionalNetworks).toEqual([]);
  });

  it("adds, edits and removes additional networks", () => {
    const { result } = renderHook(() => useDockerNetworksFormState(undefined));

    act(() => result.current.addAdditionalNetwork());
    expect(result.current.additionalNetworks).toHaveLength(1);

    act(() => result.current.updateAdditionalNetwork(0, { name: MACVLAN }));
    expect(result.current.additionalNetworks[0].name).toBe(MACVLAN);

    act(() => result.current.updateAdditionalNetwork(0, { gwPriority: "-10" }));
    expect(result.current.additionalNetworks[0]).toEqual({
      name: "lan-macvlan",
      gwPriority: "-10",
    });

    act(() => result.current.removeAdditionalNetwork(0));
    expect(result.current.additionalNetworks).toEqual([]);
  });

  it("restores the saved values on reset", () => {
    const { result } = renderHook(() =>
      useDockerNetworksFormState({
        docker_network: PRIMARY,
        docker_additional_networks: `[{"name":"${MACVLAN}"}]`,
      }),
    );

    act(() => {
      result.current.setPrimaryNetwork("other-bridge");
      result.current.addAdditionalNetwork();
    });
    expect(result.current.primaryNetwork).toBe("other-bridge");
    expect(result.current.additionalNetworks).toHaveLength(2);

    act(() => result.current.resetDockerNetworks());

    expect(result.current.primaryNetwork).toBe(PRIMARY);
    expect(result.current.additionalNetworks).toEqual([{ name: MACVLAN, gwPriority: "" }]);
  });
});
