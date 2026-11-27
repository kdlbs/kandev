import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("measure_boot.py")
SPEC = importlib.util.spec_from_file_location("measure_boot", MODULE_PATH)
measure_boot = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(measure_boot)


class BootMeasurementTests(unittest.TestCase):
    def test_summarizes_legacy_payload_without_entity_table(self):
        task = {"id": "task-1", "workspace_id": "workspace-1", "workflow_id": "workflow-1"}
        session = {"id": "session-1", "task_id": "task-1", "state": "RUNNING"}
        payload = {
            "initialState": {
                "kanban": {"tasks": [task]},
                "kanbanMulti": {"snapshots": {"workflow-1": {"tasks": [task]}}},
                "taskSessions": {"items": {"session-1": session}},
            },
        }

        row = measure_boot.summarize_payload(payload, "/", 0, 1.0, 100)

        self.assertEqual(row["boot_version"], 1)
        self.assertEqual(row["task_entities"], 1)
        self.assertEqual(row["session_entities"], 1)
        self.assertEqual(row["board_tasks"], 1)
        self.assertEqual(row["snapshot_tasks"], 1)
        self.assertEqual(row["detail_session_rows"], 1)

    def test_summarizes_normalized_payload_memberships_and_entities(self):
        payload = {
            "version": 2,
            "initialState": {
                "kanban": {"taskIds": ["task-1"]},
                "kanbanMulti": {"snapshots": {"workflow-1": {"taskIds": ["task-1"]}}},
                "taskSessions": {"sessionIds": ["session-1"]},
            },
            "routeData": {"taskDetail": {"sessionId": "session-1"}},
            "entities": {
                "tasks": {"task-1": {"id": "task-1"}},
                "sessions": {"session-1": {"id": "session-1"}},
            },
        }

        row = measure_boot.summarize_payload(payload, "/", 0, 1.0, 100)

        self.assertEqual(row["boot_version"], 2)
        self.assertEqual(row["task_entities"], 1)
        self.assertEqual(row["session_entities"], 1)
        self.assertEqual(row["board_tasks"], 1)
        self.assertEqual(row["snapshot_tasks"], 1)
        self.assertEqual(row["detail_session_rows"], 1)
        self.assertEqual(row["detail_session"], "session-1")


if __name__ == "__main__":
    unittest.main()
