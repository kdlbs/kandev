import { expect, type Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";

export function waitForTaskList(page: Page, mode: "active" | "archived" | "all", query = "") {
  return waitForHttp(page, "GET", /\/api\/v1\/workspaces\/[^/]+\/tasks$/, {
    predicate: (response) => {
      const params = new URL(response.url()).searchParams;
      let archive = "active";
      if (params.get("only_archived") === "true") archive = "archived";
      else if (params.get("include_archived") === "true") archive = "all";
      return archive === mode && (params.get("query") ?? "") === query;
    },
  });
}

export async function chooseArchiveCommand(page: Page, touch = false) {
  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Command Palette", exact: true });
  await expect(palette).toBeVisible();
  await palette.getByRole("combobox").fill("Go to archived tasks");
  const command = palette
    .getByRole("option")
    .filter({ has: page.getByText("Go to archived tasks", { exact: true }) });
  await expect(command).toHaveCount(1);
  const loaded = waitForTaskList(page, "archived");
  if (touch) await command.tap();
  else await page.keyboard.press("Enter");
  await loaded;
  await expect(palette).toBeHidden();
  await expect(page.getByTestId("tasks-list-archive-archived")).toHaveAttribute("data-state", "on");
}

export async function selectTaskListFilter(
  page: Page,
  field: "workflow" | "repository",
  name: string,
  touch = false,
) {
  const trigger = page.getByTestId(`tasks-list-${field}-filter`);
  if (touch) await trigger.tap();
  else await trigger.click();
  const option = page.getByRole("listbox").getByRole("option", { name, exact: true });
  if (touch) await option.tap();
  else await option.click();
  await expect(page.getByRole("listbox")).toHaveCount(0);
}
