import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

export function workflowStepsResponse(page: Page, workflowId: string) {
  return page.waitForResponse((response) =>
    response.url().includes("/api/v1/workflows/" + workflowId + "/workflow/steps"),
  );
}

export function armWorkflowStepPreviewResponses(page: Page, workflows: Array<{ id: string }>) {
  return workflows.map(({ id }) => workflowStepsResponse(page, id));
}

export async function expectWorkflowStepPreviewsLoaded(
  page: Page,
  workflows: Array<{ id: string; stepNames: string[] }>,
  responses: ReturnType<typeof armWorkflowStepPreviewResponses>,
) {
  const receivedResponses = await Promise.all(responses);
  for (const response of receivedResponses) expect(response.ok()).toBe(true);
  await Promise.all(workflows.map(({ id, stepNames }) => expectStepsInOrder(page, id, stepNames)));
}

export async function expectStepsInOrder(page: Page, workflowId: string, stepNames: string[]) {
  const group = page.getByTestId("workflow-option-steps-" + workflowId);
  await expect(group).toBeVisible();
  await expect(group.locator("span.wrap-anywhere")).toHaveText(stepNames);
}

export async function expectUnbrokenStepToFitGroup(
  page: Page,
  workflowId: string,
  stepName: string,
) {
  const text = page
    .getByTestId("workflow-option-steps-" + workflowId)
    .getByText(stepName, { exact: true });
  await expect(text).toBeVisible();
  const layout = await text.evaluate((element) => {
    const group = element.closest<HTMLElement>("[data-testid^='workflow-option-steps-']");
    if (!group) throw new Error("Workflow step text is outside its step group");
    const range = document.createRange();
    range.selectNodeContents(element);
    const groupBox = group.getBoundingClientRect();
    return {
      group: {
        left: groupBox.left,
        right: groupBox.right,
      },
      text: Array.from(range.getClientRects(), (rect) => ({
        left: rect.left,
        right: rect.right,
      })),
    };
  });
  expect(layout.text.length).toBeGreaterThan(1);
  for (const line of layout.text) {
    expect(line.left).toBeGreaterThanOrEqual(layout.group.left - 1);
    expect(line.right).toBeLessThanOrEqual(layout.group.right + 1);
  }
}

export type WorkflowStepPreviewScenario = {
  taskId: string;
  kanban: { id: string; stepNames: string[] };
  feature: { id: string; stepNames: string[]; unbrokenStepName: string };
  review: { id: string; stepNames: string[] };
  extraWorkflows: Array<{ id: string; stepName: string }>;
  allWorkflows: Array<{ id: string; name: string; stepNames: string[] }>;
};

type WorkflowStepPreviewScenarioOptions = {
  extraWorkflowCount?: number;
  longWorkflowSteps?: boolean;
};

const overflowStageNames = [
  "Architecture consultation and product boundary review",
  "Backend domain model and persistence development",
  "Frontend task creation and workflow picker development",
  "Security and authorization review",
  "External integration and contract verification",
  "Error handling and recovery path review",
  "Release notes and artifact preparation",
  "Accessibility and keyboard navigation check",
  "Mobile touch target and viewport behavior review",
  "Automated regression and compatibility verification",
  "Deployment readiness and rollback preparation",
  "Operations runbook and observability review",
  "Performance and large inventory verification",
  "Documentation and user guidance update",
  "Final approval and release coordination",
];

function getOverflowStageNames(workflowName: string): string[] {
  return overflowStageNames.map((name, index) => `${workflowName}: ${name} ${index + 1}`);
}

export async function seedWorkflowStepPreviewScenario(
  apiClient: ApiClient,
  workspaceId: string,
  { extraWorkflowCount = 0, longWorkflowSteps = false }: WorkflowStepPreviewScenarioOptions = {},
): Promise<WorkflowStepPreviewScenario> {
  const kanban = await apiClient.createWorkflow(workspaceId, "Preview Kanban");
  const feature = await apiClient.createWorkflow(workspaceId, "Feature Plan");
  const review = await apiClient.createWorkflow(workspaceId, "Contributor Review");
  const unbrokenStepName = "workflow-reference-" + "x".repeat(100);
  const featureSteps = longWorkflowSteps
    ? getOverflowStageNames("Feature Plan")
    : [
        "Analysis",
        unbrokenStepName,
        "Implementation checkpoint with a long review note and enough detail to wrap on a phone",
        "Review",
        "Done",
      ];
  const workflowSeeds = [
    {
      workflow: kanban,
      stepNames: longWorkflowSteps
        ? getOverflowStageNames("Preview Kanban")
        : ["Backlog", "In Progress", "Review", "Done"],
    },
    { workflow: feature, stepNames: featureSteps },
    {
      workflow: review,
      stepNames: longWorkflowSteps
        ? getOverflowStageNames("Contributor Review")
        : ["Triage", "Prepare contribution", "Maintainer review", "Complete"],
    },
  ];

  for (const { workflow, stepNames } of workflowSeeds) {
    for (const [position, name] of stepNames.entries()) {
      await apiClient.createWorkflowStep(workflow.id, name, position, {
        is_start_step: position === 0,
      });
    }
  }

  const extraWorkflows: Array<{ id: string; stepName: string }> = [];
  const allWorkflows = workflowSeeds.map(({ workflow, stepNames }) => ({
    id: workflow.id,
    name: workflow.name,
    stepNames,
  }));
  for (let index = 0; index < extraWorkflowCount; index += 1) {
    const workflowName = `Picker overflow ${index + 1}`;
    const workflow = await apiClient.createWorkflow(workspaceId, workflowName);
    const stepNames = longWorkflowSteps
      ? getOverflowStageNames(workflowName)
      : [`Overflow start ${index + 1}`];
    for (const [position, name] of stepNames.entries()) {
      await apiClient.createWorkflowStep(workflow.id, name, position, {
        is_start_step: position === 0,
      });
    }
    extraWorkflows.push({ id: workflow.id, stepName: stepNames[0] });
    allWorkflows.push({ id: workflow.id, name: workflow.name, stepNames });
  }

  const kanbanSteps = await apiClient.listWorkflowSteps(kanban.id);
  const kanbanStartStepName = workflowSeeds[0].stepNames[0]!;
  const startStepId = kanbanSteps.steps.find((step) => step.name === kanbanStartStepName)?.id;
  if (!startStepId) throw new Error("Preview Kanban has no starting step");
  const task = await apiClient.seedTask(workspaceId, "Workflow preview navigation task", {
    workflow_id: kanban.id,
    workflow_step_id: startStepId,
  });

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: kanban.id,
    task_create_last_used: {
      workflow_ids_by_workspace: { [workspaceId]: kanban.id },
    },
  });

  return {
    taskId: task.task_id,
    kanban: { id: kanban.id, stepNames: workflowSeeds[0].stepNames },
    feature: { id: feature.id, stepNames: featureSteps, unbrokenStepName },
    review: {
      id: review.id,
      stepNames: workflowSeeds[2].stepNames,
    },
    extraWorkflows,
    allWorkflows,
  };
}

export async function cleanupWorkflowStepPreviewScenario(
  apiClient: ApiClient,
  scenario: WorkflowStepPreviewScenario,
): Promise<void> {
  for (const workflowId of [
    ...scenario.extraWorkflows.map(({ id }) => id),
    scenario.review.id,
    scenario.feature.id,
    scenario.kanban.id,
  ]) {
    await apiClient.deleteWorkflow(workflowId).catch(() => {});
  }
}

export async function expectWorkflowPickerOverflow(
  page: Page,
  workflows: WorkflowStepPreviewScenario["allWorkflows"],
): Promise<{ popover: Locator; optionList: Locator }> {
  const popover = page.getByTestId("workflow-selector-popover");
  const optionList = page.getByTestId("workflow-selector-option-list");
  const popoverBox = await popover.boundingBox();
  if (!popoverBox) throw new Error("Workflow selector has no layout box");
  const viewport = page.viewportSize();
  if (!viewport) throw new Error("Workflow selector test has no viewport");
  expect(popoverBox.x).toBeGreaterThanOrEqual(0);
  expect(popoverBox.y).toBeGreaterThanOrEqual(0);
  expect(popoverBox.x + popoverBox.width).toBeLessThanOrEqual(viewport.width);
  expect(popoverBox.y + popoverBox.height).toBeLessThanOrEqual(viewport.height);

  const geometry = await optionList.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
    overflowY: getComputedStyle(element).overflowY,
  }));
  expect(geometry.overflowY).toBe("auto");
  expect(geometry.scrollHeight).toBeGreaterThan(geometry.clientHeight);
  expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth + 1);

  const previewLayouts = await optionList.evaluate(
    (element, workflowIds) => {
      const wanted = new Set(workflowIds);
      return Array.from(
        element.querySelectorAll<HTMLElement>("[data-testid^='workflow-option-steps-']"),
      )
        .filter((group) =>
          wanted.has(group.dataset.testid?.slice("workflow-option-steps-".length) ?? ""),
        )
        .map((group) => ({
          clientWidth: group.clientWidth,
          scrollWidth: group.scrollWidth,
          left: group.getBoundingClientRect().left,
          right: group.getBoundingClientRect().right,
        }));
    },
    workflows.map(({ id }) => id),
  );
  expect(previewLayouts).toHaveLength(workflows.length);
  for (const layout of previewLayouts) {
    expect(layout.scrollWidth).toBeLessThanOrEqual(layout.clientWidth + 1);
    expect(layout.left).toBeGreaterThanOrEqual(popoverBox.x);
    expect(layout.right).toBeLessThanOrEqual(popoverBox.x + popoverBox.width);
  }

  const documentWidth = await page.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }));
  expect(documentWidth.scroll).toBeLessThanOrEqual(documentWidth.client);
  return { popover, optionList };
}

export async function getWorkflowPickerEndOptions(
  page: Page,
  optionList: Locator,
  workflows: WorkflowStepPreviewScenario["allWorkflows"],
): Promise<{ first: Locator; last: Locator }> {
  const expectedIds = workflows.map(({ id }) => id);
  const renderedIds = await optionList
    .locator("button[data-testid^='workflow-option-select-']")
    .evaluateAll((buttons) =>
      buttons
        .map((button) =>
          button.getAttribute("data-testid")?.slice("workflow-option-select-".length),
        )
        .filter((id): id is string => Boolean(id)),
    );
  expect(renderedIds).toHaveLength(expectedIds.length);
  expect(renderedIds).toEqual(expect.arrayContaining(expectedIds));
  return {
    first: optionList.getByTestId(`workflow-option-select-${renderedIds[0]!}`),
    last: optionList.getByTestId(`workflow-option-select-${renderedIds.at(-1)!}`),
  };
}

export async function expectWorkflowOptionVisibleAndHitTestable(
  page: Page,
  optionList: Locator,
  option: Locator,
): Promise<{ x: number; y: number }> {
  const optionBox = await option.boundingBox();
  const listBox = await optionList.boundingBox();
  const testId = await option.getAttribute("data-testid");
  if (!optionBox || !listBox || !testId) throw new Error("Workflow option is not measurable");
  expect(optionBox.width).toBeGreaterThanOrEqual(44);
  expect(optionBox.height).toBeGreaterThanOrEqual(44);
  expect(optionBox.x).toBeGreaterThanOrEqual(listBox.x);
  expect(optionBox.x + optionBox.width).toBeLessThanOrEqual(listBox.x + listBox.width);
  const visibleTop = Math.max(optionBox.y, listBox.y);
  const visibleBottom = Math.min(optionBox.y + optionBox.height, listBox.y + listBox.height);
  expect(visibleBottom).toBeGreaterThan(visibleTop);
  const point = {
    x: optionBox.x + optionBox.width / 2,
    y: (visibleTop + visibleBottom) / 2,
  };
  const hit = await page.evaluate(
    ({ x, y, targetTestId }) =>
      Boolean(document.elementFromPoint(x, y)?.closest(`[data-testid="${targetTestId}"]`)),
    { ...point, targetTestId: testId },
  );
  expect(hit).toBe(true);
  return point;
}

async function getVisibleWorkflowStepPoint(
  optionList: Locator,
  workflowIds: string[],
): Promise<{ x: number; y: number }> {
  const point = await optionList.evaluate((element, ids) => {
    const wanted = new Set(ids);
    const listBox = element.getBoundingClientRect();
    const groups = Array.from(
      element.querySelectorAll<HTMLElement>("[data-testid^='workflow-option-steps-']"),
    ).filter((group) =>
      wanted.has(group.dataset.testid?.slice("workflow-option-steps-".length) ?? ""),
    );
    for (const group of groups) {
      const text = group.querySelector<HTMLElement>(".wrap-anywhere");
      if (!text) continue;
      const range = document.createRange();
      range.selectNodeContents(text);
      for (const rect of Array.from(range.getClientRects())) {
        const left = Math.max(rect.left, listBox.left);
        const right = Math.min(rect.right, listBox.right);
        const top = Math.max(rect.top, listBox.top);
        const bottom = Math.min(rect.bottom, listBox.bottom);
        if (right > left && bottom > top) {
          return { x: (left + right) / 2, y: (top + bottom) / 2 };
        }
      }
    }
    return null;
  }, workflowIds);
  if (!point) throw new Error("No workflow step text is visible inside the option list");
  return point;
}

export async function expectSmallAndDiagonalWorkflowWheelDeltas(
  page: Page,
  optionList: Locator,
  workflowIds: string[],
): Promise<void> {
  await optionList.evaluate((element) => {
    element.scrollTop = 0;
  });
  const point = await getVisibleWorkflowStepPoint(optionList, workflowIds);
  await page.mouse.move(point.x, point.y);
  const beforeSmallDelta = await optionList.evaluate((element) => element.scrollTop);
  await page.mouse.wheel(0, 35);
  await expect
    .poll(() => optionList.evaluate((element) => element.scrollTop))
    .toBeGreaterThan(beforeSmallDelta);

  const beforeDiagonalDelta = await optionList.evaluate((element) => element.scrollTop);
  await page.mouse.wheel(45, 70);
  await expect
    .poll(() => optionList.evaluate((element) => element.scrollTop))
    .toBeGreaterThan(beforeDiagonalDelta);
  await expect(optionList).toHaveJSProperty("scrollLeft", 0);
}

export async function wheelWorkflowOptionListToBoundary(
  page: Page,
  optionList: Locator,
  direction: "up" | "down",
): Promise<void> {
  const box = await optionList.boundingBox();
  if (!box) throw new Error("Workflow option list has no layout box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const state = await optionList.evaluate((element) => ({
      top: element.scrollTop,
      bottom: element.scrollHeight - element.clientHeight,
    }));
    if (direction === "down" ? state.bottom - state.top <= 1 : state.top <= 1) break;
    await page.mouse.wheel(0, direction === "down" ? 560 : -560);
    if (direction === "down") {
      await expect
        .poll(() => optionList.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(state.top);
    } else {
      await expect
        .poll(() => optionList.evaluate((element) => element.scrollTop))
        .toBeLessThan(state.top);
    }
  }
  const finalTop = await optionList.evaluate((element) => element.scrollTop);
  const maxTop = await optionList.evaluate(
    (element) => element.scrollHeight - element.clientHeight,
  );
  expect(direction === "down" ? finalTop : maxTop - finalTop).toBeGreaterThanOrEqual(maxTop - 1);
}

export async function touchScrollWorkflowOptionList(
  page: Page,
  optionList: Locator,
  direction: "up" | "down",
): Promise<void> {
  const box = await optionList.boundingBox();
  if (!box || box.height < 48)
    throw new Error("Workflow option list is too small for a touch swipe");
  const x = box.x + box.width / 2;
  const startY = box.y + box.height * (direction === "down" ? 0.78 : 0.22);
  const endY = box.y + box.height * (direction === "down" ? 0.22 : 0.78);
  const client = await page.context().newCDPSession(page);
  let touchStarted = false;
  try {
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x, y: startY, id: 1 }],
    });
    touchStarted = true;
    for (let step = 1; step <= 6; step += 1) {
      const y = startY + ((endY - startY) * step) / 6;
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y, id: 1 }],
      });
    }
  } finally {
    if (touchStarted) {
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    }
    await client.detach();
  }
}

export async function touchWorkflowOptionListToBoundary(
  page: Page,
  optionList: Locator,
  direction: "up" | "down",
): Promise<void> {
  const initialState = await optionList.evaluate((element) => ({
    top: element.scrollTop,
    bottom: element.scrollHeight - element.clientHeight,
  }));
  expect(initialState.bottom).toBeGreaterThan(1);
  expect(
    direction === "down" ? initialState.bottom - initialState.top : initialState.top,
  ).toBeGreaterThan(1);

  let gestureCount = 0;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const state = await optionList.evaluate((element) => ({
      top: element.scrollTop,
      bottom: element.scrollHeight - element.clientHeight,
    }));
    if (direction === "down" ? state.bottom - state.top <= 1 : state.top <= 1) break;
    await touchScrollWorkflowOptionList(page, optionList, direction);
    gestureCount += 1;
    if (direction === "down") {
      await expect
        .poll(() => optionList.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(state.top);
    } else {
      await expect
        .poll(() => optionList.evaluate((element) => element.scrollTop))
        .toBeLessThan(state.top);
    }
  }
  expect(gestureCount).toBeGreaterThan(0);
  const finalTop = await optionList.evaluate((element) => element.scrollTop);
  const maxTop = await optionList.evaluate(
    (element) => element.scrollHeight - element.clientHeight,
  );
  expect(direction === "down" ? finalTop : maxTop - finalTop).toBeGreaterThanOrEqual(maxTop - 1);
}
