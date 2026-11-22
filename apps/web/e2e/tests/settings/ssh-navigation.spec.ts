import { test } from "../../fixtures/test-base";
import {
  exerciseSSHNavigation,
  exerciseSSHSidebar,
  exerciseSSHBookmark,
  exerciseSSHSearch,
  exerciseSSHPseudo,
} from "../../helpers/ssh-navigation";

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.6, AC-EXECUTORS-SSH-CONNECTIONS-005.6
for (const width of [767, 768, 1280]) {
  test(`nested SSH navigation and bookmarks at ${width}px`, async ({ backend, testPage }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
    try {
      await testPage.setViewportSize({ width, height: 900 });
      await exerciseSSHNavigation(testPage, width < 768);
      await exerciseSSHSidebar(testPage, width < 768);
      await exerciseSSHBookmark(testPage);
      await exerciseSSHSearch(testPage, width < 768);
      await exerciseSSHPseudo(testPage);
    } finally {
      await testPage.close();
      await release();
    }
  });
}
