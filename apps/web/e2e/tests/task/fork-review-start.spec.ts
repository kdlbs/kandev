import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { verifyForkReviewStart } from "./fork-review-start-helpers";

useRegularMode();

test("warns before starting a watched fork review", async ({ testPage, apiClient, seedData }) => {
  await verifyForkReviewStart({ testPage, apiClient, seedData }, false);
});
