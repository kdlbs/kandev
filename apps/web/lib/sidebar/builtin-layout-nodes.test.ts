import { describe, expect, it } from "vitest";
import {
  BUILTIN_LAYOUT_NODES,
  builtinLayoutNodeLabelKey,
  isBuiltinLayoutNodeEligible,
  type BuiltinLayoutContext,
} from "./builtin-layout-nodes";

const node = (destinationId: string) =>
  BUILTIN_LAYOUT_NODES.find((entry) => entry.destinationId === destinationId)!;

function context(overrides: Partial<BuiltinLayoutContext> = {}): BuiltinLayoutContext {
  return {
    hasWorkspace: true,
    inOffice: false,
    features: { coordinator: false, canvases: false, needsYouInbox: false },
    ...overrides,
  };
}

describe("builtin sidebar layout nodes", () => {
  it("keeps Coordinator available only with its feature and an active workspace", () => {
    expect(isBuiltinLayoutNodeEligible(node("coordinators"), context())).toBe(false);
    expect(
      isBuiltinLayoutNodeEligible(
        node("coordinators"),
        context({ features: { coordinator: true, canvases: false, needsYouInbox: false } }),
      ),
    ).toBe(true);
    expect(
      isBuiltinLayoutNodeEligible(
        node("coordinators"),
        context({
          hasWorkspace: false,
          features: { coordinator: true, canvases: false, needsYouInbox: false },
        }),
      ),
    ).toBe(false);
    expect(
      isBuiltinLayoutNodeEligible(
        node("coordinators"),
        context({
          inOffice: true,
          features: { coordinator: true, canvases: false, needsYouInbox: false },
        }),
      ),
    ).toBe(true);
  });

  it("uses feature flags instead of resource-list contents for Inbox and Canvases", () => {
    const enabled = context({
      features: { coordinator: false, canvases: true, needsYouInbox: true },
    });
    expect(isBuiltinLayoutNodeEligible(node("canvases"), enabled)).toBe(true);
    expect(isBuiltinLayoutNodeEligible(node("needs_you_inbox"), enabled)).toBe(true);
    expect(
      isBuiltinLayoutNodeEligible(
        node("canvases"),
        context({
          inOffice: true,
          features: { coordinator: false, canvases: true, needsYouInbox: true },
        }),
      ),
    ).toBe(false);
  });

  it("keeps Office Inbox and regular workspace tools in their existing modes", () => {
    expect(isBuiltinLayoutNodeEligible(node("inbox"), context())).toBe(false);
    expect(isBuiltinLayoutNodeEligible(node("inbox"), context({ inOffice: true }))).toBe(true);
    expect(isBuiltinLayoutNodeEligible(node("automations"), context())).toBe(true);
    expect(isBuiltinLayoutNodeEligible(node("integrations"), context())).toBe(true);
    expect(isBuiltinLayoutNodeEligible(node("automations"), context({ inOffice: true }))).toBe(
      false,
    );
  });

  it("labels Needs-you Inbox for its Office destination", () => {
    expect(builtinLayoutNodeLabelKey(node("needs_you_inbox"), context())).toBe("sidebar:inbox");
    expect(builtinLayoutNodeLabelKey(node("needs_you_inbox"), context({ inOffice: true }))).toBe(
      "sidebar:needsYouInbox",
    );
  });
});
