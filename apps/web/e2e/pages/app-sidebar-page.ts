import { type Locator, type Page } from "@playwright/test";

/**
 * Page object for the unified AppSidebar (`<aside data-testid="app-sidebar">`).
 *
 * Post-overhaul, the office nav (Agents, Projects) lives in COLLAPSIBLE
 * `AppSidebarSection`s. Per `lib/state/slices/ui/app-sidebar-actions.ts`
 * (DEFAULT_SECTION_EXPANDED) the `agents` and `projects` sections default to
 * COLLAPSED, and `app-sidebar.tsx`'s SECTION_ROUTE_MAP only auto-expands them on
 * the `/office/agents` / `/office/projects` routes — NOT on the `/office`
 * dashboard. So on `/office` the Agents section is collapsed and its agent rows
 * (e.g. the CEO `<Link>`) are not rendered until the section header is expanded.
 *
 * Each section header (`app-sidebar-section.tsx` -> `SectionHeader`) renders a
 * `<button aria-expanded>` whose accessible name is the section label
 * ("Agents" / "Projects" / "Tasks"). Tests that assert section rows must expand
 * the relevant section first via {@link expandSection}.
 */
export class AppSidebarPage {
  readonly root: Locator;

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("app-sidebar");
  }

  /**
   * Expand a collapsible sidebar section by its label (e.g. "Agents",
   * "Projects") if it is not already expanded. The header toggle is a
   * `<button>` with `aria-expanded` whose accessible name is the label.
   * Idempotent: a no-op when the section is already open.
   */
  async expandSection(label: string): Promise<void> {
    const header = this.root.getByRole("button", { name: label, exact: true }).first();
    await header.waitFor({ state: "visible", timeout: 10_000 });
    if ((await header.getAttribute("aria-expanded")) !== "true") {
      await header.click();
    }
  }
}
