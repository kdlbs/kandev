import { cleanup, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { activateLocale } from "@/lib/i18n";
import { UpdateChannelControl } from "./update-channel-control";

describe("UpdateChannelControl reasons under the pseudo-locale", () => {
  beforeAll(async () => activateLocale("pseudo"));
  afterAll(async () => activateLocale("en"));
  afterEach(cleanup);

  it.each([
    "Nightly updates require a Kandev-managed npm or npx user service.",
    "Nightly updates require valid Kandev service metadata.",
    "Nightly updates are only available for user services.",
    "This service manager does not support nightly updates.",
    "Nightly updates are only available for npm or npx installations.",
    "Update channel settings are unavailable.",
    "An unknown server capability reason.",
  ])("localizes the server reason: %s", (unsupportedReason) => {
    render(
      <UpdateChannelControl
        draft="stable"
        editable={false}
        isDirty={false}
        isSaving={false}
        unsupportedReason={unsupportedReason}
        setDraft={vi.fn()}
      />,
    );

    const reason = screen.getByTestId("system-updates-channel-reason").textContent ?? "";
    expect(reason).not.toContain(unsupportedReason);
    expect(reason).toMatch(/[À-ɏ]/);
  });
});
