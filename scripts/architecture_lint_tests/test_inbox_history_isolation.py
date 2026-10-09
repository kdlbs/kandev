"""Tests for ARCH-INBOX-HISTORY-ISOLATION."""

from support import ArchitectureFixture, INBOX_HISTORY_ISOLATION_RULE


class InboxHistoryIsolationSinkTest(ArchitectureFixture):
    def test_sink_file_referencing_read_entry_point_is_flagged(self) -> None:
        path = "apps/backend/internal/task/repository/sqlite/pending_interactions.go"
        self.write(
            path,
            """
            package sqlite

            func buildPendingInteractionQuery() {
                bundles, _ := ListInboxHistoryBundles(nil, nil)
                _ = bundles
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 4)
        self.assertIn(INBOX_HISTORY_ISOLATION_RULE, result.stdout)
        self.assertIn("ListInboxHistoryBundles", result.stdout)

    def test_sink_file_referencing_count_entry_point_is_flagged(self) -> None:
        path = "apps/backend/internal/task/service/service_events.go"
        self.write(
            path,
            """
            package service

            func addTaskPendingActionEventField() {
                total, _ := CountInboxHistoryBundles(nil, nil)
                _ = total
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("CountInboxHistoryBundles", result.stdout)

    def test_unrelated_backend_file_is_not_scanned(self) -> None:
        path = "apps/backend/internal/task/repository/sqlite/clarification_bundle_query.go"
        self.write(
            path,
            """
            package sqlite

            func unrelated() {
                bundles, _ := ListInboxHistoryBundles(nil, nil)
                _ = bundles
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout)


class InboxHistoryIsolationSourceTest(ArchitectureFixture):
    def test_history_hook_referencing_needs_you_selector_is_flagged(self) -> None:
        path = "apps/web/hooks/domains/inbox-history/use-inbox-history-controller.ts"
        self.write(
            path,
            """
            import { selectNeedsYouInboxCount } from "@/lib/state/slices/needs-you-inbox/selectors";
            export function useInboxHistoryController() {
              return selectNeedsYouInboxCount;
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 1)
        self.assertIn(INBOX_HISTORY_ISOLATION_RULE, result.stdout)
        self.assertIn("selectNeedsYouInboxCount", result.stdout)

    def test_history_slice_reaching_into_needs_you_state_is_flagged(self) -> None:
        path = "apps/web/lib/state/slices/inbox-history/inbox-history-slice.ts"
        self.write(
            path,
            """
            export function bumpCount(state: any) {
              return state.needsYouInbox.byWorkspaceId;
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("needsYouInbox.byWorkspaceId", result.stdout)

    def test_history_slice_calling_needs_you_write_action_is_flagged(self) -> None:
        path = "apps/web/lib/state/slices/inbox-history/inbox-history-slice.ts"
        self.write(
            path,
            """
            import { setNeedsYouInboxPage } from "@/lib/state/slices/needs-you-inbox/needs-you-inbox-slice";
            export function bumpPage(state: any) {
              return setNeedsYouInboxPage(state);
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("setNeedsYouInboxPage", result.stdout)

    def test_history_presentation_module_is_scanned(self) -> None:
        path = "apps/web/lib/inbox-history/row-presentation.ts"
        self.write(
            path,
            """
            import { selectNeedsYouInboxCount } from "@/lib/state/slices/needs-you-inbox/selectors";
            export const x = selectNeedsYouInboxCount;
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 1)
        self.assertIn("selectNeedsYouInboxCount", result.stdout)

    def test_history_types_file_is_scanned(self) -> None:
        path = "apps/web/lib/types/inbox-history.ts"
        self.write(
            path,
            """
            export const x = "needsYouInbox.byWorkspaceId";
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("needsYouInbox.byWorkspaceId", result.stdout)

    def test_history_component_subscribing_to_pending_action_event_is_flagged(self) -> None:
        path = "apps/web/components/inbox-history/inbox-history-list.tsx"
        self.write(
            path,
            """
            export function wire(client: any) {
              return client.on("session.pending_action_changed", () => {});
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("session.pending_action_changed", result.stdout)

    def test_history_api_file_subscribing_to_state_changed_event_is_flagged(self) -> None:
        path = "apps/web/lib/api/domains/inbox-history-api.ts"
        self.write(
            path,
            """
            export function wire(client: any) {
              return client.on("session.state_changed", () => {});
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("session.state_changed", result.stdout)

    def test_backend_history_read_file_referencing_event_target_is_flagged(self) -> None:
        path = "apps/backend/internal/task/repository/sqlite/clarification_history_query.go"
        self.write(
            path,
            """
            package sqlite

            // session.pending_action_changed
            func placeholder() {}
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("session.pending_action_changed", result.stdout)

    def test_history_own_bundle_count_is_not_flagged(self) -> None:
        """AC .16 requires History to render its own bundle count, so the
        bare identifier "count" must never be a forbidden target -- only the
        Needs-you slice's own symbols are."""
        path = "apps/web/components/inbox-history/inbox-history-tab.tsx"
        self.write(
            path,
            """
            export function InboxHistoryTab({ count }: { count: number }) {
              return count;
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout)

    def test_unrelated_web_file_is_not_scanned(self) -> None:
        path = "apps/web/components/needs-you-inbox/needs-you-inbox-row.tsx"
        self.write(
            path,
            """
            import { selectNeedsYouInboxCount } from "@/lib/state/slices/needs-you-inbox/selectors";
            export const x = selectNeedsYouInboxCount;
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout)
