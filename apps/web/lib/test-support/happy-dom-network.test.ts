import type { Window as HappyDOMWindow } from "happy-dom";
import { expect, it } from "vitest";

it("disables external stylesheet loads without dispatching an error", () => {
  const settings = (window as unknown as HappyDOMWindow).happyDOM.settings;

  expect(settings.disableCSSFileLoading).toBe(true);
  expect(settings.handleDisabledFileLoadingAsSuccess).toBe(true);
});

it("does not navigate main-frame test fixtures over the network", () => {
  const settings = (window as unknown as HappyDOMWindow).happyDOM.settings;

  expect(settings.navigation.disableMainFrameNavigation).toBe(true);
});
