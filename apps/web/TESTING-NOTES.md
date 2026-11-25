## Web testing notes

- Keep `process.env.NODE_ENV = "test"` in `vitest.config.ts`. React's production build does not provide the `act()` required by the testing library. The setup preflight and CI guard this setting.
- For E2E tests, arm a causal HTTP or WebSocket wait before the action. Await the event before asserting the UI with its default timeout. Use `expect.poll` for backend state. Call `watchWs(page)` before the first `page.goto()`.
- Use `dwell` with its category and reason when no event exists. Use `injectLatency` inside route handlers when latency is the stimulus. Do not use raw sleeps or `page.waitForTimeout`. See `e2e/README.md` for signatures, examples, and enforcement. Never narrow the sleep guard.
- In jsdom, intercept secure-cookie setters. Test Radix tooltips with keyboard focus; use Playwright for pointer hover.
- Scope terminal selectors to the active panel. Mobile and dockview can mount multiple instances.
- Use the configured worker budget for full Vitest suites. Do not overlap suites or override the worker limit.
