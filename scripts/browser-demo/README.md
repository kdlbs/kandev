# Browser demo

The browser demo runs the normal Kandev SPA against a stateful demo API in a
Web Worker. It has no backend, VM, container, or external network dependency.

The scenario lives in `apps/web/lib/browser-demo/scenario.ts`. The Worker owns
task, session, message, and pull-request state and persists mutations in the
browser. Normal web builds do not activate the demo runtime.

Build it with:

```sh
./scripts/browser-demo/build-web-demo.sh
```

The default output is `apps/web/dist-browser-demo`. Pass a destination as the
first argument when a release workflow needs to stage the bundle for another
repository.

Supported demo behavior:

- seeded workspace, repository, workflow, tasks, sessions, and GitHub PR
- create, edit, move, archive, and delete tasks
- start the mock agent and stream its first response
- restore browser-local scenario state after a reload

Host-only operations return HTTP 501 with `demo_mode: true`.

