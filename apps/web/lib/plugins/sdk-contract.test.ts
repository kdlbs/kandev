import { describe, expect, it } from "vitest";
import type {
  ChatTopBarSlotProps as PublicChatTopBarSlotProps,
  ChatSubmitDecorationSlotProps as PublicChatSubmitDecorationSlotProps,
  HostReact as PublicHostReact,
  MainTopBarSlotProps as PublicMainTopBarSlotProps,
  PluginConversationApi as PublicPluginConversationApi,
  PluginConversationError as PublicPluginConversationError,
  PluginConversationMessage as PublicPluginConversationMessage,
  PluginConversationTurn as PublicPluginConversationTurn,
  PluginHostApi as PublicPluginHostApi,
  PluginNavSection as PublicPluginNavSection,
  PluginRegistry as PublicPluginRegistry,
  PluginSessionMessagesQuery as PublicPluginSessionMessagesQuery,
  PluginSessionMessagesState as PublicPluginSessionMessagesState,
  PluginSessionTurnsState as PublicPluginSessionTurnsState,
  PluginTaskPanelContext as PublicPluginTaskPanelContext,
  PluginTaskPanelProps as PublicPluginTaskPanelProps,
  PluginUIApi as PublicPluginUIApi,
  RepositoryProviderRegistration as PublicRepositoryProviderRegistration,
  ReviewSummary as PublicReviewSummary,
  ReviewTaskAssociation as PublicReviewTaskAssociation,
  TaskMenuActionRegistration as PublicTaskMenuActionRegistration,
  TaskMenuSubItemRegistration as PublicTaskMenuSubItemRegistration,
  TaskPanelRegistration as PublicTaskPanelRegistration,
} from "@kandev/plugin-sdk";
import type {
  ChatTopBarSlotProps as HostChatTopBarSlotProps,
  ChatSubmitDecorationSlotProps as HostChatSubmitDecorationSlotProps,
  MainTopBarSlotProps as HostMainTopBarSlotProps,
  PluginConversationApi,
  PluginConversationError,
  PluginConversationMessage,
  PluginConversationTurn,
  PluginHostApi,
  PluginNavSection,
  PluginRegistry,
  PluginSessionMessagesQuery,
  PluginSessionMessagesState,
  PluginSessionTurnsState,
  PluginTaskPanelContext,
  PluginTaskPanelProps,
  RepositoryProviderRegistration,
  ReviewItemSummary,
  ReviewTaskAssociation,
  TaskMenuActionRegistration,
  TaskMenuSubItemRegistration,
  TaskPanelRegistration,
} from "./types";

type MissingPublicHostKeys = Exclude<keyof PublicPluginHostApi, keyof PluginHostApi>;
type MissingPublicRegistryKeys = Exclude<keyof PublicPluginRegistry, keyof PluginRegistry>;
type MissingHostRegistryKeys = Exclude<keyof PluginRegistry, keyof PublicPluginRegistry>;
type RuntimeSatisfiesPublicHost = PluginHostApi extends PublicPluginHostApi ? true : false;
type RuntimeSatisfiesPublicRegistry = PluginRegistry extends PublicPluginRegistry ? true : false;
type SameType<Left, Right> = Left extends Right ? (Right extends Left ? true : false) : false;
type HasUseLayoutEffect = "useLayoutEffect" extends keyof PublicHostReact ? true : false;
type HasPromptMentionText = "PromptMentionText" extends keyof PublicPluginUIApi ? true : false;
type HasConversationApi = "conversation" extends keyof PublicPluginHostApi ? true : false;
type HasTaskMenuItems = "items" extends keyof PublicTaskMenuActionRegistration ? true : false;

// A registration that predates submenus must keep compiling unchanged, and the
// submenu shape must be expressible: that pair is the whole compatibility story
// of the optional `items` field.
const flatOnlyTaskMenuAction: PublicTaskMenuActionRegistration = {
  id: "legacy",
  label: "Legacy",
  group: "primary",
  run: () => {},
};
const submenuTaskMenuAction: PublicTaskMenuActionRegistration = {
  id: "submenu",
  label: "Submenu",
  group: "primary",
  items: () => [{ id: "child", label: "Child", run: () => {} }],
  run: () => {},
};

const legacyTaskPanelRegistration: PublicTaskPanelRegistration = {
  id: "legacy",
  title: "Legacy",
  Component: () => null,
};

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
    const chatTopBarSlotPropsAreCanonical: SameType<
      HostChatTopBarSlotProps,
      PublicChatTopBarSlotProps
    > = true;
    const chatSubmitDecorationSlotPropsAreCanonical: SameType<
      HostChatSubmitDecorationSlotProps,
      PublicChatSubmitDecorationSlotProps
    > = true;
    const conversationMessageIsCanonical: SameType<
      PluginConversationMessage,
      PublicPluginConversationMessage
    > = true;
    const conversationTurnIsCanonical: SameType<
      PluginConversationTurn,
      PublicPluginConversationTurn
    > = true;
    const conversationErrorIsCanonical: SameType<
      PluginConversationError,
      PublicPluginConversationError
    > = true;
    const messagesQueryIsCanonical: SameType<
      PluginSessionMessagesQuery,
      PublicPluginSessionMessagesQuery
    > = true;
    const messagesStateIsCanonical: SameType<
      PluginSessionMessagesState,
      PublicPluginSessionMessagesState
    > = true;
    const turnsStateIsCanonical: SameType<PluginSessionTurnsState, PublicPluginSessionTurnsState> =
      true;
    const conversationApiIsCanonical: SameType<PluginConversationApi, PublicPluginConversationApi> =
      true;
    const taskPanelContextIsCanonical: SameType<
      PluginTaskPanelContext,
      PublicPluginTaskPanelContext
    > = true;
    const taskPanelPropsAreCanonical: SameType<PluginTaskPanelProps, PublicPluginTaskPanelProps> =
      true;
    const taskPanelRegistrationIsCanonical: SameType<
      TaskPanelRegistration,
      PublicTaskPanelRegistration
    > = true;
    const publicReactHasLayoutEffect: HasUseLayoutEffect = true;
    const publicUiHasPromptMentionText: HasPromptMentionText = true;
    const publicHostHasConversation: HasConversationApi = true;
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
    expect(chatTopBarSlotPropsAreCanonical).toBe(true);
    expect(chatSubmitDecorationSlotPropsAreCanonical).toBe(true);
    expect(conversationMessageIsCanonical).toBe(true);
    expect(conversationTurnIsCanonical).toBe(true);
    expect(conversationErrorIsCanonical).toBe(true);
    expect(messagesQueryIsCanonical).toBe(true);
    expect(messagesStateIsCanonical).toBe(true);
    expect(turnsStateIsCanonical).toBe(true);
    expect(conversationApiIsCanonical).toBe(true);
    expect(taskPanelContextIsCanonical).toBe(true);
    expect(taskPanelPropsAreCanonical).toBe(true);
    expect(taskPanelRegistrationIsCanonical).toBe(true);
    expect(publicReactHasLayoutEffect).toBe(true);
    expect(publicUiHasPromptMentionText).toBe(true);
    expect(publicHostHasConversation).toBe(true);
    expect(legacyTaskPanelRegistration.title).toBe("Legacy");
    expect(publicHostContract).toBeTypeOf("function");
    expect(publicRegistryContract).toBeTypeOf("function");
  });
});

// The registrations this feature adds to the public SDK. A plugin author
// imports these names verbatim, and the host's runtime-facing types must stay
// identical to them, including the optional `items` field.
describe("task menu submenu SDK contract", () => {
  it("is canonical in both directions", () => {
    const taskMenuActionRegistrationIsCanonical: SameType<
      TaskMenuActionRegistration,
      PublicTaskMenuActionRegistration
    > = true;
    const taskMenuSubItemRegistrationIsCanonical: SameType<
      TaskMenuSubItemRegistration,
      PublicTaskMenuSubItemRegistration
    > = true;
    const publicTaskMenuActionHasItems: HasTaskMenuItems = true;
    expect(taskMenuActionRegistrationIsCanonical).toBe(true);
    expect(taskMenuSubItemRegistrationIsCanonical).toBe(true);
    expect(publicTaskMenuActionHasItems).toBe(true);
    // A registration that predates submenus still compiles unchanged, and the
    // submenu shape is expressible: that pair is the compatibility story of the
    // optional `items` field.
    expect(flatOnlyTaskMenuAction.run).toBeTypeOf("function");
    expect(submenuTaskMenuAction.items).toBeTypeOf("function");
  });
});
