/**
 * Native JS UI bundle for the kandev-plugin-e2e fixture plugin
 * (docs/plans/plugins/PLUGIN-API.md). A self-contained ES module — no
 * imports, no bundled React — that calls `window.registerKandevPlugin(id,
 * plugin)` at evaluation time and, on `initialize(registry, host)`,
 * registers a nav item, a top-level route, a `task-sidebar` slot component,
 * a `main-top-bar` slot component, a `task.created` WS handler, and the
 * `open-demo` keybinding (declared in manifest.yaml's `ui.keybindings`,
 * default `mod+shift+k`) which opens a `host.openModal(...)` demo modal.
 * Uses only host.React/host.jsx.
 *
 * The task-created counter lives in module scope (not component state) with
 * a tiny listener set, so it survives across route navigations (the page
 * component unmounts/remounts as the user navigates away and back).
 */
(function () {
  var moduleCount = 0;
  var listeners = new Set();

  function emit() {
    listeners.forEach(function (fn) {
      fn(moduleCount);
    });
  }

  function incrementCount() {
    moduleCount += 1;
    emit();
  }

  function useCounter(React) {
    var state = React.useState(moduleCount);
    var count = state[0];
    var setCount = state[1];
    React.useEffect(function () {
      setCount(moduleCount);
      listeners.add(setCount);
      return function () {
        listeners.delete(setCount);
      };
    }, []);
    return count;
  }

  var PROVIDER_ID = "fixture-source-control";
  var PULL_REQUEST_URL =
    "https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/42";
  var REPOSITORY_URL = "https://bitbucket.example.test/scm/TEAM/fixture.git";

  function fixtureRepository() {
    return {
      id: "fixture-repository",
      repositoryId: "fixture-repository",
      owner: "TEAM",
      ownerOrProject: "TEAM",
      name: "fixture",
      repositoryName: "fixture",
      fullName: "TEAM/fixture",
      url: REPOSITORY_URL,
      cloneUrl: REPOSITORY_URL,
      providerHost: "bitbucket.example.test",
      defaultBranch: "main",
      private: true,
    };
  }

  function abortableRefresh(signal) {
    return new Promise(function (_resolve, reject) {
      if (signal.aborted) {
        reject(new Error("fixture review refresh aborted"));
        return;
      }
      signal.addEventListener(
        "abort",
        function () {
          reject(new Error("fixture review refresh aborted"));
        },
        { once: true },
      );
    });
  }

  window.registerKandevPlugin("kandev-plugin-e2e", {
    initialize: function (registry, host) {
      var React = host.React;
      var jsx = host.jsx;

      function PluginPage() {
        var count = useCounter(React);
        var connectionState = React.useState("Not checked");
        var connection = connectionState[0];
        var setConnection = connectionState[1];
        function checkConnection() {
          host.api
            .invokeAction("connection-status", {
              workspaceId: host.store.getState().workspaces.activeId || undefined,
            })
            .then(function (result) {
              setConnection(result.connected ? "Connected" : result.error || "Connection unavailable");
            })
            .catch(function (error) {
              setConnection(error instanceof Error ? error.message : "Connection unavailable");
            });
        }
        return jsx(
          "div",
          { id: "hello-plugin-page-root" },
          jsx("h1", { id: "hello-plugin-page" }, "Hello E2E"),
          jsx("span", { id: "hello-task-counter" }, String(count)),
          jsx(
            "button",
            {
              id: "fixture-connection-status",
              "data-testid": "fixture-connection-status",
              type: "button",
              onClick: checkConnection,
            },
            "Check Bitbucket connection",
          ),
          jsx("span", { id: "fixture-connection-result", "data-testid": "fixture-connection-result" }, connection),
        );
      }

      function FixtureReviewPanel(props) {
        return jsx(
          "section",
          {
            "data-testid": "fixture-review-panel-" + props.presentation,
            "data-review-key": props.reviewKey,
          },
          jsx(
            "h2",
            null,
            "Bitbucket pull request #" + props.reviewKey.replace("pull-request-", ""),
          ),
          jsx("p", null, "Provider-neutral fixture review panel"),
        );
      }

      function FixtureReviewSelector() {
        return jsx("span", { "data-testid": "fixture-review-selector" }, "Bitbucket");
      }

      function openLinkResult(context) {
        host.openTaskLinkDialog({
          title: "Link Bitbucket pull request",
          description: "Use a Bitbucket pull request URL for this task.",
          inputLabel: "Pull request",
          placeholder: PULL_REQUEST_URL,
          emptyError: "Enter a Bitbucket pull request URL.",
          failureMessage: "Failed to link Bitbucket pull request.",
          successMessage: "Bitbucket pull request linked",
          inputTestId: "fixture-link-pull-request-input",
          errorTestId: "fixture-link-pull-request-error",
          submitTestId: "fixture-link-pull-request-submit",
          onSubmit: function (reference) {
            return host.api
              .invokeAction("link-pull-request", {
                workspaceId: context.workspaceId,
                taskId: context.taskId,
                body: { pullRequestUrl: reference },
              })
              .then(function (result) {
                if (!result.linked) {
                  throw new Error(result.error || "Connection unavailable");
                }
              });
          },
        });
        return Promise.resolve();
      }

      function SidebarSlot() {
        return jsx("div", { id: "hello-sidebar" }, "Hello E2E sidebar");
      }

      function MainTopBarSlot(props) {
        var slotProps = props.slotProps || {};
        return jsx("span", { id: "hello-main-top-bar" }, "Hello " + slotProps.currentPage);
      }

      function StatusSlot(props) {
        var slotProps = props.slotProps || {};
        var id = slotProps.placement === "left" ? "hello-status-left" : "hello-status-right";
        return jsx(
          "span",
          { id: id },
          "Hello status " +
            String(slotProps.presentation || "unknown") +
            " " +
            String(slotProps.activeTaskId || "no-task"),
        );
      }

      registry.registerNavItem({
        id: "e2e-hello",
        label: "Hello E2E",
        path: "/plugins/e2e-hello",
        section: "main",
      });
      registry.registerRoute("/plugins/e2e-hello", PluginPage);
      registry.registerComponent("task-sidebar", SidebarSlot);
      registry.registerComponent("main-top-bar", MainTopBarSlot);
      registry.registerComponent("app-status-bar-left", StatusSlot);
      registry.registerComponent("app-status-bar-right", StatusSlot);
      registry.registerWsHandler("task.created", function () {
        incrementCount();
      });

      registry.registerRepositoryProvider({
        id: PROVIDER_ID,
        label: "Bitbucket",
        listRepositories: function () {
          return Promise.resolve([fixtureRepository()]);
        },
        matchesURL: function (url) {
          if (typeof url !== "string") return false;
          try {
            return new URL(url).hostname === "bitbucket.example.test";
          } catch (_error) {
            return false;
          }
        },
        listBranches: function (_context) {
          return Promise.resolve([{ name: "main" }, { name: "feature/provider-contract" }]);
        },
        inspectURL: function (_context) {
          return Promise.resolve({
            providerId: PROVIDER_ID,
            providerHost: "bitbucket.example.test",
            ownerOrProject: "TEAM",
            repositoryId: "fixture-repository",
            repositoryName: "fixture",
            cloneUrl: REPOSITORY_URL,
            defaultBranch: "main",
            baseBranch: "main",
            headBranch: "feature/provider-contract",
            pullRequest: { number: 42, title: "Provider-neutral contract" },
          });
        },
      });

      registry.registerTaskAction({
        id: "link-bitbucket-pull-request",
        label: "Bitbucket Pull Request",
        icon: "bitbucket",
        placement: "link",
        group: "Link",
        run: openLinkResult,
      });

      registry.registerReviewProvider({
        id: PROVIDER_ID,
        label: "Bitbucket",
        changeRequestNoun: "Pull Request",
        order: 50,
        getSnapshot: function (taskId) {
          return [
            {
              providerId: PROVIDER_ID,
              reviewKey: "pull-request-42",
              title: "Bitbucket Pull Request #42",
              url: PULL_REQUEST_URL,
              repositoryId: "fixture-repository",
              state: "OPEN",
              statusBadge: { label: "Open" },
              taskId: taskId,
            },
            {
              providerId: PROVIDER_ID,
              reviewKey: "pull-request-43",
              title: "Bitbucket Pull Request #43",
              url: "https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/43",
              repositoryId: "fixture-repository",
              state: "OPEN",
              statusBadge: { label: "Open" },
              taskId: taskId,
            },
          ];
        },
        subscribe: function () {
          return function () {};
        },
        refresh: function (_taskId, signal) {
          return abortableRefresh(signal);
        },
        ReviewPanel: FixtureReviewPanel,
        Selector: FixtureReviewSelector,
      });

      registry.registerKeybinding("open-demo", function () {
        function DemoModalContent() {
          return jsx(
            "div",
            { id: "hello-demo-modal", "data-testid": "hello-demo-modal" },
            "Hello from the plugin modal",
          );
        }
        host.openModal({
          title: "Demo Modal",
          content: DemoModalContent,
          size: "md",
        });
      });
    },
  });
})();
