package messagequeue

import "expvar"

var routineWakeFullBoardScansSuppressed = expvar.NewInt("message_queue_routine_full_board_scans_suppressed_total")

func recordRoutineWakeSuppressed() {
	routineWakeFullBoardScansSuppressed.Add(1)
}
