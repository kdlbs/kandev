# Collect Debug Logs

Use this guide when a maintainer asks for frontend or backend logs while investigating a Kandev issue.

Debug logs can include prompts, tool calls, file paths, task IDs, repository names, and snippets of local context. Review the files before sharing them publicly. Prefer sending them in a private support thread when they may contain sensitive data.

## Which Logs to Send

A maintainer may ask for one kind of log or for both:

- **Frontend logs**: Browser console output from the affected page. These are usually enough for UI issues, layout problems, stale page state, missing updates, or click flows that behave incorrectly.
- **Backend logs**: Terminal or service logs from the Kandev server. These are useful for startup failures, agent failures, task execution problems, integration errors, database errors, and service issues.
- **Both**: Best when the issue crosses the UI and server boundary, such as WebSocket updates, task/session state mismatches, missing messages, or anything that is hard to classify.

If the maintainer asks for frontend logs only, you do not need to send `~/kandev-debug.log` unless they ask for it later.

## Quick Steps

1. Stop the Kandev server you are already using.
   - If it is running in a terminal, press `Ctrl-C`.
   - If it is running as a user service, run `kandev service stop`.
   - If it is running as a system service, run `sudo kandev service stop --system`.
2. Start Kandev in debug mode using the same install channel:

   ```bash
   # Homebrew or global npm install
   kandev --debug 2>&1 | tee ~/kandev-debug.log
   ```

   ```bash
   # npx install
   npx -y kandev@latest --debug 2>&1 | tee ~/kandev-debug.log
   ```

3. Open the task or page that has the issue.
4. Open the browser DevTools Console, clear it, enable verbose/debug messages, then refresh the page.
5. Reproduce the issue.
6. Share:
   - The browser console export or screenshot/text, if frontend logs were requested.
   - `~/kandev-debug.log`, if backend logs were requested.
   - The task URL or task ID, and a short description of what you clicked before the issue happened.
7. When done, press `Ctrl-C` in the debug terminal. If you stopped a service, start it again:

   ```bash
   kandev service start
   ```

   ```bash
   sudo kandev service start --system
   ```

## Frontend Console Logs

The frontend debug logs are browser console logs. They are most useful when they only contain the page and task that has the issue.

1. Start Kandev with `--debug`.
2. Open the affected task details page.
3. Open DevTools.
   - Chrome or Edge: right-click the page, choose **Inspect**, then open **Console**.
   - Firefox: right-click the page, choose **Inspect**, then open **Console**.
4. In the Console:
   - Clear existing logs.
   - Enable **Verbose** or **Debug** logs. Kandev frontend debug lines use `console.debug`.
   - Enable **Preserve log** if the issue involves page reloads or navigation.
   - Leave the filter box empty unless a maintainer asks for a specific filter.
5. Refresh the page.
6. Reproduce the issue.
7. Copy or export the Console output and share it with the maintainer.

You can check whether frontend debug mode is active by typing this in the Console:

```js
window.__KANDEV_DEBUG
```

It should print `true`.

## Backend Logs

When Kandev is started with:

```bash
kandev --debug 2>&1 | tee ~/kandev-debug.log
```

the backend logs are written to the terminal and saved to `~/kandev-debug.log`.

Leave that terminal open while reproducing the issue. After the issue happens, send the saved file.

## Service Installs

If you normally run Kandev as a service, the cleanest debug flow is usually:

```bash
kandev service stop
kandev --debug 2>&1 | tee ~/kandev-debug.log
# reproduce the issue
# press Ctrl-C
kandev service start
```

For a system service:

```bash
sudo kandev service stop --system
kandev --debug 2>&1 | tee ~/kandev-debug.log
# reproduce the issue
# press Ctrl-C
sudo kandev service start --system
```

If you should not stop the service, collect the existing service logs instead:

```bash
kandev service logs > ~/kandev-service.log
```

```bash
sudo kandev service logs --system > ~/kandev-service.log
```

To capture live logs while reproducing:

```bash
kandev service logs -f | tee ~/kandev-service.log
```

```bash
sudo kandev service logs -f --system | tee ~/kandev-service.log
```

## Improve Kandev Log Bundle

Kandev also has a built-in path for sharing recent logs with a task:

1. Start Kandev with `--debug`.
2. Reproduce the issue in the same browser tab.
3. Open **Improve Kandev** from the sidebar.
4. Leave **Include recent backend & browser logs as context for the agent** checked.
5. Submit the issue.

This writes recent backend logs and browser console events to a temporary bundle and adds the bundle file paths to the task description.

## What to Send

When reporting an issue, include:

- Kandev version: `kandev --version` or the version shown in Settings.
- Install method: Homebrew, npx, npm global, service, Docker, or source checkout.
- Operating system.
- The task URL or task ID.
- The requested logs: frontend console logs, backend logs, or both.
- The exact steps that reproduce the issue.

Do not send your SQLite database, repository contents, credentials, API keys, or screenshots containing secrets unless a maintainer explicitly asks for them and you are comfortable sharing them privately.
