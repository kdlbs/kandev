import { describe, expect, it } from "vitest";
import type {
  PluginHostApi as PublicPluginHostApi,
  ChatSubmitDecorationSlotProps as PublicChatSubmitDecorationSlotProps,
  MainTopBarSlotProps as PublicMainTopBarSlotProps,
  PluginNavSection as PublicPluginNavSection,
  PluginRegistry as PublicPluginRegistry,
  RepositoryProviderRegistration as PublicRepositoryProviderRegistration,
  ReviewSummary as PublicReviewSummary,
  ReviewTaskAssociation as PublicReviewTaskAssociation,
} from "@kandev/plugin-sdk";
import type {
  PluginHostApi,
  ChatSubmitDecorationSlotProps as HostChatSubmitDecorationSlotProps,
  MainTopBarSlotProps as HostMainTopBarSlotProps,
  PluginNavSection,
  PluginRegistry,
  RepositoryProviderRegistration,
  ReviewItemSummary,
  ReviewTaskAssociation,
} from "./types";

type MissingPublicHostKeys = Exclude<keyof PublicPluginHostApi, keyof PluginHostApi>;
type MissingPublicRegistryKeys = Exclude<keyof PublicPluginRegistry, keyof PluginRegistry>;
type MissingHostRegistryKeys = Exclude<keyof PluginRegistry, keyof PublicPluginRegistry>;
type RuntimeSatisfiesPublicHost = PluginHostApi extends PublicPluginHostApi ? true : false;
type RuntimeSatisfiesPublicRegistry = PluginRegistry extends PublicPluginRegistry ? true : false;
type SameType<Left, Right> = Left extends Right ? (Right extends Left ? true : false) : false;

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
    const repositoryProviderIsCanonical: SameType<
      RepositoryProviderRegistration,
      PublicRepositoryProviderRegistration
    > = true;
    const reviewSummaryIsCanonical: SameType<ReviewItemSummary, PublicReviewSummary> = true;
    const associationIsCanonical: SameType<ReviewTaskAssociation, PublicReviewTaskAssociation> =
      true;
    // Named-import equality, not just structural: a plugin author writes
    // `import type { PluginNavSection } from "@kandev/plugin-sdk"` verbatim, and
    // TS's structural typing can't otherwise tell an inline union from a named export.
    const navSectionIsCanonical: SameType<PluginNavSection, PublicPluginNavSection> = true;
    const mainTopBarSlotPropsAreCanonical: SameType<
      HostMainTopBarSlotProps,
      PublicMainTopBarSlotProps
    > = true;
    const chatSubmitDecorationSlotPropsAreCanonical: SameType<
      HostChatSubmitDecorationSlotProps,
      PublicChatSubmitDecorationSlotProps
    > = true;
    expect(hostHasEveryPublicKey).toBe(true);
    expect(registryHasEveryPublicKey).toBe(true);
    expect(publicHasEveryRegistryKey).toBe(true);
    expect(runtimeSatisfiesPublicHost).toBe(true);
    expect(runtimeSatisfiesPublicRegistry).toBe(true);
    expect(repositoryProviderIsCanonical).toBe(true);
    expect(reviewSummaryIsCanonical).toBe(true);
    expect(associationIsCanonical).toBe(true);
    expect(navSectionIsCanonical).toBe(true);
    expect(mainTopBarSlotPropsAreCanonical).toBe(true);
    expect(chatSubmitDecorationSlotPropsAreCanonical).toBe(true);
    expect(publicHostContract).toBeTypeOf("function");
    expect(publicRegistryContract).toBeTypeOf("function");
  });
});
