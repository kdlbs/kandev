import { test } from "../../fixtures/test-base";
import { updateThemeFromMarketplace } from "./theme-marketplace-update-flow";

test("phone updates a theme from Installed with verified release identity", async ({
  testPage,
  apiClient,
}) => {
  await updateThemeFromMarketplace(testPage, apiClient);
});
