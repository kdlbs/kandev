import { describe, expect, it } from "vitest";
import {
  APP_STATUS_CONNECTION_ID,
  APP_STATUS_METRICS_ID,
  moveAppStatusItem,
  projectActiveStatusItems,
  reconcileAppStatusBarOrder,
  resolveAppStatusDropTarget,
  type AppStatusItemDescriptor,
} from "./app-status-bar-order";

const PLUGIN_LEFT_ID = "plugin:left";
const PLUGIN_RIGHT_ID = "plugin:right";
const DORMANT_ID = "dormant";
const DORMANT_RIGHT_ID = "dormant-right";

const items: AppStatusItemDescriptor[] = [
  { id: APP_STATUS_CONNECTION_ID, defaultSide: "left" },
  { id: PLUGIN_LEFT_ID, defaultSide: "left" },
  { id: APP_STATUS_METRICS_ID, defaultSide: "right" },
  { id: PLUGIN_RIGHT_ID, defaultSide: "right" },
];

describe("reconcileAppStatusBarOrder", () => {
  it("builds the default two-sided order when no preference exists", () => {
    expect(reconcileAppStatusBarOrder({ leftItemIds: [], rightItemIds: [] }, items)).toEqual({
      leftItemIds: [APP_STATUS_CONNECTION_ID, PLUGIN_LEFT_ID],
      rightItemIds: [APP_STATUS_METRICS_ID, PLUGIN_RIGHT_ID],
    });
  });

  it("keeps saved cross-spacer order and dormant plugin identities", () => {
    const saved = {
      leftItemIds: [PLUGIN_RIGHT_ID, DORMANT_ID, PLUGIN_RIGHT_ID],
      rightItemIds: [APP_STATUS_CONNECTION_ID, DORMANT_ID],
    };

    expect(reconcileAppStatusBarOrder(saved, items)).toEqual({
      leftItemIds: [PLUGIN_RIGHT_ID, DORMANT_ID, PLUGIN_LEFT_ID],
      rightItemIds: [APP_STATUS_CONNECTION_ID, APP_STATUS_METRICS_ID],
    });
  });

  it("projects only active items and restores a re-enabled item at its saved position", () => {
    const saved = {
      leftItemIds: [PLUGIN_LEFT_ID, DORMANT_ID, APP_STATUS_CONNECTION_ID],
      rightItemIds: [APP_STATUS_METRICS_ID],
    };
    const reconciled = reconcileAppStatusBarOrder(saved, items);

    expect(projectActiveStatusItems(reconciled, items).left.map((item) => item.id)).toEqual([
      PLUGIN_LEFT_ID,
      APP_STATUS_CONNECTION_ID,
    ]);
    const reenabled = [...items, { id: DORMANT_ID, defaultSide: "right" as const }];
    expect(projectActiveStatusItems(reconciled, reenabled).left.map((item) => item.id)).toEqual([
      PLUGIN_LEFT_ID,
      DORMANT_ID,
      APP_STATUS_CONNECTION_ID,
    ]);
  });
});

describe("moveAppStatusItem", () => {
  it("moves an active item across the spacer without dropping dormant identities", () => {
    const order = {
      leftItemIds: [APP_STATUS_CONNECTION_ID, "dormant-left", PLUGIN_LEFT_ID],
      rightItemIds: [APP_STATUS_METRICS_ID, DORMANT_RIGHT_ID, PLUGIN_RIGHT_ID],
    };

    expect(moveAppStatusItem(order, PLUGIN_LEFT_ID, "right", 1, items)).toEqual({
      leftItemIds: [APP_STATUS_CONNECTION_ID, "dormant-left"],
      rightItemIds: [APP_STATUS_METRICS_ID, PLUGIN_LEFT_ID, DORMANT_RIGHT_ID, PLUGIN_RIGHT_ID],
    });
  });

  it("inserts after the last active item but before trailing dormant identities", () => {
    const order = {
      leftItemIds: [APP_STATUS_CONNECTION_ID],
      rightItemIds: [APP_STATUS_METRICS_ID, PLUGIN_RIGHT_ID, DORMANT_RIGHT_ID],
    };

    expect(moveAppStatusItem(order, APP_STATUS_CONNECTION_ID, "right", 2, items)).toEqual({
      leftItemIds: [],
      rightItemIds: [
        APP_STATUS_METRICS_ID,
        PLUGIN_RIGHT_ID,
        APP_STATUS_CONNECTION_ID,
        DORMANT_RIGHT_ID,
      ],
    });
  });
});

describe("resolveAppStatusDropTarget", () => {
  const geometry = [
    { id: "left-a", side: "left" as const, left: 0, right: 20 },
    { id: "left-b", side: "left" as const, left: 20, right: 40 },
    { id: "right-a", side: "right" as const, left: 80, right: 100 },
  ];

  it("uses the spacer midpoint to choose a side and item centers for insertion", () => {
    expect(resolveAppStatusDropTarget(5, geometry, { left: 40, right: 80 })).toEqual({
      side: "left",
      activeIndex: 0,
    });
    expect(resolveAppStatusDropTarget(35, geometry, { left: 40, right: 80 })).toEqual({
      side: "left",
      activeIndex: 2,
    });
    expect(resolveAppStatusDropTarget(70, geometry, { left: 40, right: 80 })).toEqual({
      side: "right",
      activeIndex: 0,
    });
    expect(resolveAppStatusDropTarget(95, geometry, { left: 40, right: 80 })).toEqual({
      side: "right",
      activeIndex: 1,
    });
  });
});
