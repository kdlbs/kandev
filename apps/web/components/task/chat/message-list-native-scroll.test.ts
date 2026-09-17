import { describe, expect, it } from "vitest";
import { resolveCompetingInitialScrollOwner } from "./message-list-native-scroll";

const noProgrammaticLock = () => false;

describe("resolveCompetingInitialScrollOwner", () => {
  it("preserves layout and explicit message placement precedence", () => {
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: true,
        hasExplicitScrollTarget: true,
        hasUnreadDivider: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("layout-restore");
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: true,
        hasUnreadDivider: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("explicit-target");
  });

  it("uses the unread divider and then the programmatic owner for ordinary placement", () => {
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: false,
        hasUnreadDivider: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("unread-divider");
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: false,
        hasUnreadDivider: false,
        isProgrammaticScrollLocked: () => true,
      }),
    ).toBe("programmatic-scroll");
  });
});
