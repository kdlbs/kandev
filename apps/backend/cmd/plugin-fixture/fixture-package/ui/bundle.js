/**
 * Native JS UI bundle for the kandev-plugin-e2e fixture plugin
 * (docs/plans/plugins/PLUGIN-API.md). A self-contained ES module — no
 * imports, no bundled React — that calls `window.registerKandevPlugin(id,
 * plugin)` at evaluation time and, on `initialize(registry, host)`,
 * registers a nav item, a top-level route, a `task-sidebar` slot component,
 * a `main-top-bar` slot component, a `task.created` WS handler, and the
 * `open-demo` keybinding (declared in manifest.yaml's `ui.keybindings`,
 * default `mod+shift+j`) which opens a `host.openModal(...)` demo modal
 * containing a `host.ui` Tooltip. Uses only host.React/host.jsx/host.ui.
 *
 * The plugin page also renders a `host.theme` readout kept current purely
 * through `host.onThemeChange`, so an e2e can flip the app theme for real and
 * prove the subscription fires in a browser (jsdom's MutationObserver is a
 * shim, and jsdom cannot open a Radix tooltip from synthetic hover at all),
 * plus a button that fires `host.toast.error` against the real sonner
 * instance the app mounts.
 *
 * It also registers the plugin-hooks surface this fixture exists to drive
 * end to end: a "Notes" task panel (registerTaskPanel, mobile-enabled) that
 * round-trips a single per-user document through host.storage
 * (get on mount, debounced set, subscribe to pick up a write from another
 * tab/surface), task-card indicator/tag components, generic task-row metadata,
 * and task-menu actions under the "edit" and "primary" groups. It registers a
 * task-list facet (registerTaskListFacet) whose values a spec drives through
 * `window.__e2eFacetValues`, so /tasks facet sort and grouping can be exercised
 * against real plugin registrations. It also
 * registers one composer action on all three composer slots
 * (chat-input-actions, task-create-input-actions, new-session-input-actions),
 * which is how the e2e suite exercises PluginComposerCapability against real
 * native composers rather than a mock.
 *
 * The task-created counter lives in module scope (not component state) with
 * a tiny listener set, so it survives across route navigations (the page
 * component unmounts/remounts as the user navigates away and back).
 */
(function () {
  var moduleCount = 0;
  var listeners = new Set();
  // Task-list facet plumbing. Values come from `window.__e2eFacetValues`
  // (taskId -> [{value,label,color}]) so a spec can drive multi-value
  // membership, colors, and the untagged fallback against task ids the
  // fixture cannot know ahead of time. `window.__e2eFacetNotify()` fires
  // the subscription so the reactive re-read path is exercised for real.
  var facetListeners = new Set();

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
      var ui = host.ui;

      // Reads host.theme once on mount, then tracks it purely through
      // host.onThemeChange — so the readout only stays correct if the
      // subscription actually fires. Also counts notifications, proving the
      // host does not re-notify on unrelated <html> class churn.
      function useHostTheme() {
        var state = React.useState(function () {
          return { theme: host.theme, changes: 0 };
        });
        var value = state[0];
        var setValue = state[1];
        React.useEffect(function () {
          return host.onThemeChange(function (next) {
            setValue(function (prev) {
              return { theme: next, changes: prev.changes + 1 };
            });
          });
        }, []);
        return value;
      }

      function PluginPage() {
        var count = useCounter(React);
        var connectionState = React.useState("Not checked");
        var connection = connectionState[0];
        var setConnection = connectionState[1];
        var themeState = useHostTheme();
        function checkConnection() {
          host.api
            .invokeAction("connection-status", {
              workspaceId: host.store.getState().workspaces.activeId || undefined,
            })
            .then(function (result) {
              setConnection(
                result.connected ? "Connected" : result.error || "Connection unavailable",
              );
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
          jsx(
            "span",
            { id: "fixture-connection-result", "data-testid": "fixture-connection-result" },
            connection,
          ),
          jsx(
            "span",
            {
              "data-testid": "hello-theme-readout",
              "data-theme-changes": String(themeState.changes),
            },
            themeState.theme,
          ),
          // host.toast is a per-plugin Proxy over sonner; unit tests only ever
          // see it against a mocked sonner, so this button is how an e2e
          // proves a real toast renders and that .error does NOT file a
          // frontend-error report.
          jsx(
            "button",
            {
              "data-testid": "hello-toast-error",
              onClick: function () {
                host.toast.error("Plugin toast error");
              },
            },
            "toast error",
          ),
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
        var label = "Hello " + slotProps.currentPage;
        return jsx(
          ui.Button,
          {
            id: "hello-main-top-bar",
            variant: "outline",
            size: "icon-sm",
            "aria-label": label,
          },
          jsx(
            "svg",
            {
              className: "h-4 w-4",
              viewBox: "0 0 24 24",
              fill: "none",
              stroke: "currentColor",
              "aria-hidden": "true",
            },
            jsx("path", { d: "M5 12h14M12 5l7 7l-7 7" }),
          ),
          jsx("span", { className: "sr-only" }, label),
        );
      }

      // Debounce delay for the Notes panel's autosave — short, so e2e specs
      // don't need to wait long for a write to reach host.storage.
      var NOTES_SAVE_DEBOUNCE_MS = 150;

      function useNotesValue(taskId, panelId) {
        var state = React.useState("");
        var value = state[0];
        var setValue = state[1];
        var loadedState = React.useState(false);
        var loaded = loadedState[0];
        var setLoaded = loadedState[1];

        React.useEffect(
          function () {
            var cancelled = false;
            // Each notification (subscribe's refresh callback, plus the initial
            // call) starts its own host.storage.get. Two in flight at once can
            // resolve out of order, so `cancelled` alone (unmount-only) isn't
            // enough — an older read landing after a newer one would overwrite
            // it. Only the read matching the current generation may commit.
            var refreshGeneration = 0;
            function refresh() {
              var generation = ++refreshGeneration;
              host.storage.get("task", taskId, "note").then(
                function (entry) {
                  if (cancelled || generation !== refreshGeneration) return;
                  setValue(entry ? entry.value : "");
                  setLoaded(true);
                },
                function () {
                  if (cancelled || generation !== refreshGeneration) return;
                  setLoaded(true);
                },
              );
            }
            refresh();
            // Scope echo suppression to this panel instance (panelId) rather
            // than the host's shared tab-wide default writerId. The kanban
            // "Enhance notes" action below writes this same note using that
            // shared default (it has no ongoing subscription of its own to
            // protect) — if this panel used the same default, the action's
            // write would look like this panel's own echo and get silently
            // dropped instead of refreshing the panel.
            var unsubscribe = host.storage.subscribe(
              { scope: "task", scopeId: taskId, key: "note", writerId: panelId },
              refresh,
            );
            return function () {
              cancelled = true;
              unsubscribe();
            };
          },
          [taskId, panelId],
        );

        return [value, setValue, loaded];
      }

      function NotesPanel(props) {
        var taskId = props.taskId;
        var panelId = props.panelId;
        var notesValue = useNotesValue(taskId, panelId);
        var value = notesValue[0];
        var setValue = notesValue[1];
        var loaded = notesValue[2];
        var debounceRef = React.useRef(null);

        function handleChange(e) {
          var next = e.target.value;
          setValue(next);
          if (debounceRef.current) clearTimeout(debounceRef.current);
          debounceRef.current = setTimeout(function () {
            host.storage
              .set("task", taskId, "note", next, { writerId: panelId })
              .catch(function () {
                // surface the failed autosave instead of dropping it silently
              });
          }, NOTES_SAVE_DEBOUNCE_MS);
        }

        if (!loaded) {
          return jsx("div", { "data-testid": "e2e-notes-panel-loading" }, "Loading notes…");
        }
        return jsx("textarea", {
          "data-testid": "e2e-notes-panel",
          "data-presentation": props.presentation,
          value: value,
          onChange: handleChange,
        });
      }

      function CardIndicator(props) {
        var slotProps = props.slotProps || {};
        return jsx(
          "span",
          { "data-testid": "e2e-card-indicator", "data-task-id": slotProps.taskId },
          "N",
        );
      }

      function CardTags(props) {
        var slotProps = props.slotProps || {};
        return jsx(
          "span",
          { "data-testid": "e2e-card-tags", "data-task-id": slotProps.taskId },
          "tags",
        );
      }

      function RowMetadata(props) {
        var slotProps = props.slotProps || {};
        return jsx(
          "span",
          {
            "data-testid": "e2e-row-metadata",
            "data-task-id": slotProps.taskId,
            "data-surface": slotProps.surface,
          },
          "fixture metadata",
        );
      }

      function WorkspaceActionsSlot(props) {
        var slotProps = props.slotProps || {};
        return jsx(
          "button",
          {
            type: "button",
            "data-testid": "e2e-sidebar-workspace-actions",
            "data-workspace-id": slotProps.workspaceId,
            "data-workspace-label": slotProps.workspaceLabel || "",
            "data-presentation": slotProps.presentation || "unknown",
            "aria-label": "Fixture workspace action",
            className:
              slotProps.presentation === "mobile"
                ? "flex h-11 w-11 cursor-pointer items-center justify-center rounded-md border"
                : "flex h-6 w-6 cursor-pointer items-center justify-center rounded-md border",
            onClick: function (event) {
              event.currentTarget.setAttribute("data-clicked", "true");
            },
          },
          "W",
        );
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

      // Drives PluginComposerCapability through native composers. Capturing
      // the capability and using it after later renders mirrors a real voice
      // integration whose recording completes asynchronously.
      var COMPOSER_BUTTON_STYLE = {
        fontSize: "9px",
        lineHeight: "1",
        padding: "2px 3px",
        minWidth: "16px",
      };

      function ComposerAction(props) {
        var slotProps = props.slotProps || {};
        var composer = slotProps.composer;
        var statusState = React.useState("");
        var status = statusState[0];
        var setStatus = statusState[1];
        var capturedRef = React.useRef(null);

        function record(result) {
          setStatus(result && result.status ? result.status : String(result));
        }

        return jsx(
          "span",
          {
            "data-testid": "e2e-composer-action",
            "data-surface": String(slotProps.surface),
            "data-presentation": String(slotProps.presentation),
            "data-task-id": String(slotProps.taskId || ""),
            "data-session-id": String(slotProps.activeSessionId || ""),
            "data-disabled": String(Boolean(slotProps.disabled)),
            "data-submittable": String(Boolean(slotProps.submittable)),
            "data-status": status,
          },
          jsx(
            "button",
            {
              type: "button",
              "data-testid": "e2e-composer-insert",
              style: COMPOSER_BUTTON_STYLE,
              onClick: function () {
                record(composer.insertText("DICTATED"));
              },
            },
            "insert",
          ),
          jsx(
            "button",
            {
              type: "button",
              "data-testid": "e2e-composer-capture",
              style: COMPOSER_BUTTON_STYLE,
              onClick: function () {
                capturedRef.current = composer;
                setStatus("captured");
              },
            },
            "capture",
          ),
          jsx(
            "button",
            {
              type: "button",
              "data-testid": "e2e-composer-insert-captured",
              style: COMPOSER_BUTTON_STYLE,
              onClick: function () {
                record(
                  capturedRef.current
                    ? capturedRef.current.insertText("DICTATED")
                    : { status: "not-captured" },
                );
              },
            },
            "insert captured",
          ),
          jsx(
            "button",
            {
              type: "button",
              "data-testid": "e2e-composer-submit",
              style: COMPOSER_BUTTON_STYLE,
              onClick: function () {
                var target = capturedRef.current || composer;
                target.submit().then(record);
              },
            },
            "submit",
          ),
          jsx(
            "button",
            {
              type: "button",
              "data-testid": "e2e-composer-focus",
              style: COMPOSER_BUTTON_STYLE,
              onClick: function () {
                record(composer.focus());
              },
            },
            "focus",
          ),
        );
      }

      function FixtureBitbucketIcon(props) {
        return jsx(
          "svg",
          {
            className: props.className,
            viewBox: "0 0 24 24",
            fill: "none",
            stroke: "currentColor",
            "aria-hidden": props["aria-hidden"] || true,
            "data-testid": "fixture-bitbucket-icon",
          },
          jsx("path", { d: "M4 5h16l-2.5 14h-11z" }),
          jsx("path", { d: "M9 9h6l-1 6h-4z" }),
        );
      }

      registry.registerNavItem({
        id: "e2e-hello",
        label: "Hello E2E",
        path: "/plugins/e2e-hello",
        section: "main",
      });
      registry.registerNavItem({
        id: "e2e-insights-tools",
        label: "E2E Insights Tools",
        path: "/plugins/e2e-hello",
        section: "sidebar-footer",
      });
      // Three more sidebar-footer items so this one plugin install alone
      // produces P = 4 (budget MAX_INLINE_PLUGIN_FOOTER_ITEMS = 3, plus one
      // over-budget item) — enough to drive the desktop footer's overflow
      // trigger and menu with the real Radix DropdownMenu in a browser (see
      // plugins.spec.ts's overflow test). The budget counts destinations,
      // not distinct plugins, so one plugin registering 4 items exercises
      // the same partition as 4 plugins registering 1 each. Labeled
      // "E2E Overflow Item N" rather than a numbered suffix of the first
      // item's own label ("E2E Insights Tools") so Playwright's default
      // substring name matching can't accidentally match more than one of
      // these from an existing test written against the first item's label.
      registry.registerNavItem({
        id: "e2e-insights-tools-2",
        label: "E2E Overflow Item 2",
        path: "/plugins/e2e-hello",
        section: "sidebar-footer",
      });
      registry.registerNavItem({
        id: "e2e-insights-tools-3",
        label: "E2E Overflow Item 3",
        path: "/plugins/e2e-hello",
        section: "sidebar-footer",
      });
      registry.registerNavItem({
        id: "e2e-insights-tools-4",
        label: "E2E Overflow Item 4",
        path: "/plugins/e2e-hello",
        section: "sidebar-footer",
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
        icon: FixtureBitbucketIcon,
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
        icon: FixtureBitbucketIcon,
        placement: "link",
        group: "Link",
        run: openLinkResult,
      });

      registry.registerReviewProvider({
        id: PROVIDER_ID,
        label: "Bitbucket",
        icon: FixtureBitbucketIcon,
        changeRequestNoun: "Pull Request",
        order: 50,
        getSnapshot: function (taskId) {
          return [
            {
              providerId: PROVIDER_ID,
              reviewKey: "pull-request-42",
              title: "Bitbucket Pull Request #42",
              url: PULL_REQUEST_URL,
              connectionScope: "https://bitbucket.example.test",
              repositoryId: "fixture-repository",
              changeRequestNumber: 42,
              state: "OPEN",
              statusBadge: { label: "Open" },
              taskId: taskId,
            },
            {
              providerId: PROVIDER_ID,
              reviewKey: "pull-request-43",
              title: "Bitbucket Pull Request #43",
              url: "https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/43",
              connectionScope: "https://bitbucket.example.test",
              repositoryId: "fixture-repository",
              changeRequestNumber: 43,
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

      registry.registerTaskPanel({
        id: "notes",
        title: "Notes",
        icon: "book",
        Component: NotesPanel,
        mobileEnabled: true,
      });
      registry.registerTaskListFacet({
        id: "fixture-tags",
        label: "Fixture tag",
        getValues: function (context) {
          var byTask = window.__e2eFacetValues || {};
          if (byTask.__throwFor === context.taskId) throw new Error("fixture facet boom");
          return byTask[context.taskId] || [];
        },
        subscribe: function (listener) {
          facetListeners.add(listener);
          window.__e2eFacetNotify = function () {
            facetListeners.forEach(function (fn) {
              fn();
            });
          };
          return function () {
            facetListeners.delete(listener);
          };
        },
      });
      registry.registerComponent("chat-input-actions", ComposerAction);
      registry.registerComponent("task-create-input-actions", ComposerAction);
      registry.registerComponent("new-session-input-actions", ComposerAction);
      registry.registerComponent("task-card-indicators", CardIndicator);
      registry.registerComponent("task-card-tags", CardTags);
      registry.registerComponent("task-row-metadata", RowMetadata);
      registry.registerComponent("sidebar-workspace-actions", WorkspaceActionsSlot);
      registry.registerTaskMenuAction({
        id: "enhance-notes",
        label: "Enhance notes",
        group: "edit",
        run: function (context) {
          return host.storage
            .set("task", context.taskId, "note", "Enhanced via plugin action")
            .then(function () {
              // Keep the received responsive context observable to the E2E
              // fixture without changing the plugin-facing action contract.
              return host.storage.set(
                "task",
                context.taskId,
                "menu-presentation",
                context.presentation,
              );
            });
        },
      });
      registry.registerTaskMenuAction({
        id: "inspect-task-metadata",
        label: "Inspect task metadata",
        group: "primary",
        run: function (context) {
          return host.storage.set(
            "task",
            context.taskId,
            "primary-menu-presentation",
            context.presentation,
          );
        },
      });

      registry.registerKeybinding("open-demo", function () {
        function DemoModalContent() {
          var completedState = React.useState(false);
          var completed = completedState[0];
          var setCompleted = completedState[1];
          var longRows = [];
          for (var index = 0; index < 32; index += 1) {
            longRows.push(
              jsx(
                "p",
                { key: "long-modal-row-" + index },
                "Long plugin modal content row " +
                  String(index + 1) +
                  " keeps the opaque plugin surface growing beyond the viewport.",
              ),
            );
          }

          // The Tooltip is the point of this modal in e2e: PluginModalHost
          // mounts outside AppShell, so without its own TooltipProvider a
          // Tooltip here throws on render and the error boundary swallows the
          // whole modal. jsdom cannot open a Radix tooltip from synthetic
          // hover, so real pointer hover is only assertable here.
          return jsx(
            "div",
            {
              id: "hello-demo-modal",
              "data-testid": "hello-demo-modal",
              style: { display: "grid", gap: "8px" },
            },
            "Hello from the plugin modal",
            jsx(
              ui.Tooltip,
              null,
              jsx(ui.TooltipTrigger, { "data-testid": "hello-modal-tooltip-trigger" }, "hover me"),
              jsx(ui.TooltipContent, null, "Tooltip inside a plugin modal"),
            ),
            jsx("div", { "data-testid": "hello-long-modal-content" }, longRows),
            jsx(
              "button",
              {
                type: "button",
                "data-testid": "hello-long-modal-final-action",
                style: { minHeight: "44px", padding: "8px 12px" },
                onClick: function () {
                  setCompleted(true);
                },
              },
              completed ? "Plugin modal action complete" : "Complete plugin modal action",
            ),
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
