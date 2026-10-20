import { describe, expect, it } from "vitest";
import {
  resolveCompetingInitialScrollOwner,
  shouldPreserveFollowOnScroll,
} from "./message-list-native-scroll";

const noProgrammaticLock = () => false;

describe("resolveCompetingInitialScrollOwner", () => {
  it("preserves layout and explicit message placement precedence", () => {
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: true,
        hasExplicitScrollTarget: true,
        hasUnreadDivider: true,
        enabled: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("layout-restore");
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: true,
        hasUnreadDivider: true,
        enabled: true,
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
        enabled: true,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBe("unread-divider");
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: false,
        hasUnreadDivider: false,
        enabled: true,
        isProgrammaticScrollLocked: () => true,
      }),
    ).toBe("programmatic-scroll");
  });

  it("keeps saved-position placement ahead of the unread divider when auto-scroll is disabled", () => {
    expect(
      resolveCompetingInitialScrollOwner({
        hasPendingLayoutRestore: false,
        hasExplicitScrollTarget: false,
        hasUnreadDivider: true,
        enabled: false,
        isProgrammaticScrollLocked: noProgrammaticLock,
      }),
    ).toBeNull();
  });
});

describe("shouldPreserveFollowOnScroll", () => {
  it("does not treat an explicit position change as content-driven movement", () => {
    expect(
      shouldPreserveFollowOnScroll(
        { scrollTop: 1400, scrollHeight: 2000 },
        { scrollTop: 0, scrollHeight: 2000 },
      ),
    ).toBe(false);
  });

  it("keeps follow when delayed content grows without moving the viewport", () => {
    expect(
      shouldPreserveFollowOnScroll(
        { scrollTop: 800, scrollHeight: 1000 },
        { scrollTop: 800, scrollHeight: 1200 },
      ),
    ).toBe(true);
  });

  it("keeps follow when native anchoring moves with inserted content", () => {
    expect(
      shouldPreserveFollowOnScroll(
        { scrollTop: 800, scrollHeight: 1000 },
        { scrollTop: 1000, scrollHeight: 1200 },
      ),
    ).toBe(true);
  });

  it("keeps follow when native anchoring only accounts for part of inserted content", () => {
    expect(
      shouldPreserveFollowOnScroll(
        { scrollTop: 800, scrollHeight: 1000 },
        { scrollTop: 900, scrollHeight: 1200 },
      ),
    ).toBe(true);
  });

  it("does not preserve follow when scroll movement is unrelated to content growth", () => {
    expect(
      shouldPreserveFollowOnScroll(
        { scrollTop: 800, scrollHeight: 1000 },
        { scrollTop: 500, scrollHeight: 1200 },
      ),
    ).toBe(false);
  });
});
