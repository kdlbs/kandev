"""Explicit registry of architecture rules."""

from .frontend_root_state_cast import RULE as FRONTEND_ROOT_STATE_CAST
from .frontend_state_ui_import import RULE as FRONTEND_STATE_UI_IMPORT
from .deprecation_ledger import RULE as DEPRECATION_LEDGER
from .run_scheduler_owner import RULE as RUN_SCHEDULER_OWNER
from .inbox_history_isolation import RULE as INBOX_HISTORY_ISOLATION
from .runtime_import import RULE as RUNTIME_IMPORT
from .runs_office_import import RULE as RUNS_OFFICE_IMPORT
from .task_office_import import RULE as TASK_OFFICE_IMPORT


RULES = (
    RUNTIME_IMPORT,
    TASK_OFFICE_IMPORT,
    FRONTEND_ROOT_STATE_CAST,
    INBOX_HISTORY_ISOLATION,
    FRONTEND_STATE_UI_IMPORT,
    RUN_SCHEDULER_OWNER,
    RUNS_OFFICE_IMPORT,
    DEPRECATION_LEDGER,
)
