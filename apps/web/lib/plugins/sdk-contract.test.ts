import { describe, expect, it } from "vitest";
import type {
  PluginHostApi as PublicPluginHostApi,
  PluginRegistry as PublicPluginRegistry,
} from "@kandev/plugin-sdk";
import type { PluginHostApi, PluginRegistry } from "./types";

type MissingPublicHostKeys = Exclude<keyof PublicPluginHostApi, keyof PluginHostApi>;
type MissingPublicRegistryKeys = Exclude<keyof PublicPluginRegistry, keyof PluginRegistry>;
type MissingHostRegistryKeys = Exclude<keyof PluginRegistry, keyof PublicPluginRegistry>;
type RuntimeSatisfiesPublicHost = PluginHostApi extends PublicPluginHostApi ? true : false;
type RuntimeSatisfiesPublicRegistry = PluginRegistry extends PublicPluginRegistry ? true : false;

function publicHostContract(value: PluginHostApi): PublicPluginHostApi {
  return value;
}

function publicRegistryContract(value: PluginRegistry): PublicPluginRegistry {
  return value;
}
describe("public plugin SDK", () => {
  it("is satisfied by the host runtime contracts", () => {
    const hostHasEveryPublicKey: MissingPublicHostKeys extends never ? true : false = true;
    const registryHasEveryPublicKey: MissingPublicRegistryKeys extends never ? true : false = true;
    const publicHasEveryRegistryKey: MissingHostRegistryKeys extends never ? true : false = true;
    const runtimeSatisfiesPublicHost: RuntimeSatisfiesPublicHost = true;
    const runtimeSatisfiesPublicRegistry: RuntimeSatisfiesPublicRegistry = true;
    expect(hostHasEveryPublicKey).toBe(true);
    expect(registryHasEveryPublicKey).toBe(true);
    expect(publicHasEveryRegistryKey).toBe(true);
    expect(runtimeSatisfiesPublicHost).toBe(true);
    expect(runtimeSatisfiesPublicRegistry).toBe(true);
    expect(publicHostContract).toBeTypeOf("function");
    expect(publicRegistryContract).toBeTypeOf("function");
  });
});
