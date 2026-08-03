"""Explicit registry of architecture rules."""

from .frontend_root_state_cast import RULE as FRONTEND_ROOT_STATE_CAST
from .runtime_import import RULE as RUNTIME_IMPORT
from .task_office_import import RULE as TASK_OFFICE_IMPORT


RULES = (RUNTIME_IMPORT, TASK_OFFICE_IMPORT, FRONTEND_ROOT_STATE_CAST)
