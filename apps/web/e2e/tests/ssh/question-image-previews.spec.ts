import { test } from "../../fixtures/ssh-test-base";
import {
  inspectAndAnswerImages,
  openImageQuestions,
  questionImagePrepareScript,
} from "../chat/question-image-helpers";

test("question images use the real ssh workspace", async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(240_000);
  await apiClient.updateRepository(seedData.repositoryId, {
    prepare_script: questionImagePrepareScript(),
  });
  try {
    const { session } = await openImageQuestions(
      testPage,
      apiClient,
      seedData,
      seedData.sshExecutorProfileId,
    );
    await inspectAndAnswerImages(testPage, session, false);
  } finally {
    await apiClient.updateRepository(seedData.repositoryId, { prepare_script: "" });
  }
});
