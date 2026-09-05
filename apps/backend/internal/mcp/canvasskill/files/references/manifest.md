# Canvas manifest

Publish one `manifest.yaml` file at the root of the source directory returned
by `create_canvas_kandev`. Canvas packages use the current Kandev plugin
manifest schema, with `api_version: 2`. A static canvas package has no
`base_url` or managed backend endpoints.

This is a complete minimal example. The example uses a nested entry to show
that files next to the entry document can use normal relative URLs.

```yaml
id: example-canvas
api_version: 2
version: 1.0.0
display_name: Example canvas
description: A small task canvas.
author: Canvas author
ui:
  web_apps:
    - key: main
      title: Example canvas
      entry: ui/index.html
      placements:
        - task-canvas
        - workspace-canvas
      network_origins:
        - https://api.example.com
capabilities:
  api_read:
    - tasks
    - workflows
  api_write:
    - tasks
    - messages
  events:
    - task.updated
  state: true
```

The host parses YAML and calls the same manifest validator used for plugin
registration. Required identity fields are `id`, `api_version`, `version`,
`display_name`, `description`, and `author`. `api_version` must be `1` or `2`;
new canvases must use `2`.

`ui.web_apps` contains one or more applications. Each application requires a
lowercase `key`, a `title`, a clean package-relative `entry`, and at least one
placement. A placement is `task-canvas` or `workspace-canvas`. The package may
contain `ui/index.html`, `ui/app.js`, and `ui/app.css`; references such as
`./app.js` resolve beside `ui/index.html`. Do not use absolute paths, `..`,
symlinks, or a path below `_kandev`.

`network_origins` contains exact HTTPS origins only. Do not add a path,
wildcard, credentials, query, or fragment. Each origin also needs an approved
grant before the release can run. Network requests go directly from the
sandboxed browser to the approved origin. If a release, grant, archive, or
canvas authority changes, the host immediately tears down the old iframe, so
the old direct requests cannot continue under the old binding.

Capabilities are declarations, not grants. Use `api_read` for `tasks` and
`workflows`, `api_write` for `tasks` and `messages`, `events` for event
subscriptions, and `state: true` for instance state. The operator reviews the
declaration before the first release or any release that needs new grants.
Request only the capabilities used by the application.

The package must include the declared entry document and every local asset it
references. Bundle executable dependencies. A build tool, package manager, or
network build step is not available when the canvas runs.
