import { test } from "../../fixtures/docker-test-base";
import {
  inspectAndAnswerImages,
  openImageQuestions,
  questionImageSetupScript,
} from "../chat/question-image-helpers";

test("question images use the real docker workspace", async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(240_000);
  const original = await apiClient.getRepository(seedData.repositoryId);
  await apiClient.updateRepository(seedData.repositoryId, {
    setup_script: questionImageSetupScript(),
  });
  try {
    const { session } = await openImageQuestions(
      testPage,
      apiClient,
      seedData,
      seedData.dockerExecutorProfileId,
    );
    await inspectAndAnswerImages(testPage, session, false);
  } finally {
    await apiClient.updateRepository(seedData.repositoryId, {
      setup_script: original.setup_script ?? "",
    });
  }
});
