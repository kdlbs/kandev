import { test } from "../../fixtures/test-base";
import {
  exerciseSSHNavigation,
  exerciseSSHSidebar,
  exerciseSSHBookmark,
  exerciseSSHSearch,
} from "../../helpers/ssh-navigation";

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.4, AC-EXECUTORS-SSH-CONNECTIONS-004.6
for (const width of [390, 820]) {
  test(`touch SSH navigation and bookmarks at ${width}px`, async ({ backend, testPage }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
    try {
      await testPage.setViewportSize({ width, height: 900 });
      await exerciseSSHNavigation(testPage, true);
      await exerciseSSHSidebar(testPage, width < 768);
      await exerciseSSHBookmark(testPage);
      await exerciseSSHSearch(testPage, width < 768);
    } finally {
      await testPage.close();
      await release();
    }
  });
}
