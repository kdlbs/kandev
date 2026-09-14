import { describe, expect, it } from "vitest";
import { resolveCompetingInitialScrollOwner } from "./message-list-native-scroll";

const noProgrammaticLock = () => false;

describe("resolveCompetingInitialScrollOwner", () => {
  it("gives a new recovery failure the initial reveal before unread placement", () => {
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: false,
        hasRecoveryReveal: true,
        hasUnreadDivider: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("recovery");
  });

  it("preserves layout and explicit message placement precedence", () => {
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: true,
        hasExplicitScrollTarget: true,
        hasRecoveryReveal: true,
        hasUnreadDivider: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("layout-restore");
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: true,
        hasRecoveryReveal: true,
        hasUnreadDivider: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("explicit-target");
  });
});
