# Plugin examples

The canonical authoring guide is [Authoring a Plugin](public/plugins-authoring.md).
It documents the current frontend hooks, backend Host surfaces, six recipes,
package layout, and the reproducible build/package/install/test workflow.

For a maintained scaffold, start from
[`kdlbs/kandev-plugin-template`](https://github.com/kdlbs/kandev-plugin-template).
For an in-tree package smoke test, use the fixture and commands in
`apps/backend/cmd/plugin-fixture` and `make -C apps/backend e2e-plugin-package`.

The fixture is test support, not a production plugin repository. Do not rely on
old external example links that are not present in the maintained source
catalog; use the template and fixture paths above.
