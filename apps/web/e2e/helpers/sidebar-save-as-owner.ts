import { test, expect } from "../fixtures/test-base";
import { SidebarFilterPopoverPage } from "../pages/sidebar-filter-popover";

export function registerSaveAsOwnerTests() {
  for (const replacement of ["group", "base", "same owner"] as const) {
    test(`Save as naming preserves its captured owner: ${replacement}`, async ({
      testPage,
      apiClient,
      seedData,
    }, info) => {
      const workspace = seedData.workspaceId;
      const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
        workspace
      ];
      const base = { ...initial.views[0], id: "naming-base", name: "Naming base", group: "state" };
      const other = { ...base, id: "naming-other", name: "Other base" };
      const draft = {
        base_view_id: base.id,
        filters: base.filters,
        sort: base.sort,
        group: "workflow",
        collapsed_groups: [] as string[],
      };
      try {
        await apiClient.saveUserSettings({
          sidebar_view_state: {
            workspace_id: workspace,
            views_create: [base, other],
            active_view_id: base.id,
            draft,
          },
        });
        await testPage.goto(`/?workspaceId=${workspace}`);
        const mobile = Boolean(info.project.use.isMobile);
        if (mobile) await testPage.getByTestId("app-nav-trigger").tap();
        const editor = new SidebarFilterPopoverPage(testPage);
        if (mobile)
          await testPage.getByTestId("app-nav-sheet").getByTestId("sidebar-filter-gear").tap();
        else await editor.open();
        const popover = testPage.getByTestId("sidebar-filter-popover");
        if (mobile) await popover.getByTestId("view-save-as-button").tap();
        else await popover.getByTestId("view-save-as-button").click();
        const input = popover.getByTestId("view-save-as-name-input");
        await input.fill("Captured copy");
        const latest = {
          ...draft,
          base_view_id: replacement === "base" ? other.id : base.id,
          group: replacement === "group" ? "repository" : draft.group,
          collapsed_groups: ["surviving-disclosure"],
        };
        await apiClient.saveUserSettings({
          sidebar_view_state: {
            workspace_id: workspace,
            active_view_id: latest.base_view_id,
            draft: latest,
          },
        });
        if (replacement === "same owner") {
          await expect(input).toHaveValue("Captured copy");
          if (mobile) await popover.getByTestId("view-save-as-confirm").tap();
          else await input.press("Enter");
          await expect
            .poll(
              async () =>
                (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
                  workspace
                ].views.find((view) => view.name === "Captured copy")?.collapsed_groups,
            )
            .toEqual(latest.collapsed_groups);
        } else {
          await expect(input).toBeHidden();
          await expect(popover.getByTestId("view-save-as-confirm")).toHaveCount(0);
          const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
            workspace
          ];
          expect(state.views.some((view) => view.name === "Captured copy")).toBe(false);
          expect(state.draft).toMatchObject(latest);
        }
        await testPage.screenshot({
          path: info.outputPath("save-as-owner.png"),
          animations: "disabled",
        });
        await testPage.reload();
        const persisted = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
          workspace
        ];
        if (replacement === "same owner") expect(persisted.draft).toBeNull();
        else expect(persisted.draft).toMatchObject(latest);
      } finally {
        await testPage.close();
        await apiClient.saveUserSettings({
          sidebar_view_state: { workspace_id: workspace, ...initial },
        });
      }
    });
  }
}
