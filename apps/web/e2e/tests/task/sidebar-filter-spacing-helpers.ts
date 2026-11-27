import type { Locator } from "@playwright/test";

export async function measureFilterHeader(popover: Locator) {
  const label = popover.getByText("Filters", { exact: true });
  const addButton = popover.getByTestId("filter-add-button");
  const section = label.locator("xpath=../..");
  const viewRow = section.locator("xpath=preceding-sibling::*[1]");
  const [viewBox, labelBox, addBox, sectionBox] = await Promise.all([
    viewRow.boundingBox(),
    label.boundingBox(),
    addButton.boundingBox(),
    section.boundingBox(),
  ]);

  if (!viewBox || !labelBox || !addBox || !sectionBox) {
    throw new Error("Sidebar filter header geometry could not be measured");
  }

  return {
    dividerBottom: viewBox.y + viewBox.height,
    labelTop: labelBox.y,
    addBox,
    sectionBox,
  };
}
