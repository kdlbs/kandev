# Legacy action fixtures

These fixtures preserve four UI shapes from the pinned official-plugin source
audit in `docs/plans/plugin-action-ux/plugin-adoption.md`. They model the
original host Button, raw styled button, status contribution, and controlled
disclosure. Keep them as legacy React controls; do not migrate them to
`host.ui.Action` as the host implementation evolves.

| Shape                                    | Audited source                                                                                                                                    |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Host Button with copied sizing classes   | [Template `ui/bundle.js`](https://github.com/kdlbs/kandev-plugin-template/blob/be2f0c51b6fca92cf752c12f4c071961276782be/ui/bundle.js)             |
| Raw topbar button with a live metric     | [Task Manager `ui/bundle.js`](https://github.com/kdlbs/kandev-plugin-task-manager/blob/acf72a444e4a7c8e4f19519f87d95dcaa82654eb/ui/bundle.js)     |
| Status contribution with custom content  | [Provider Usage `ui/bundle.js`](https://github.com/kdlbs/kandev-plugin-provider-usage/blob/ff833ed25e11e162e9c49ac46684e9944a94f81c/ui/bundle.js) |
| Custom trigger with a controlled preview | [Kandy `ui/bundle.js`](https://github.com/kdlbs/kandev-plugin-kandy/blob/1975d8d6ebbe9b3de4e5a97d783eac4bdff54e6a/ui/bundle.js)                   |

These are attributed structural fixtures, not copied package bundles. Tests
prove representative behavior on this host; they do not certify the published
plugin artifacts or their provider integrations.
