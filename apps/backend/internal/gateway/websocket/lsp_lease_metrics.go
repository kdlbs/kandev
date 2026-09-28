package websocket

import "expvar"

var (
	lspLeaseActiveGauge   = expvar.NewInt("lsp_lease_active")
	lspLeaseDetachedGauge = expvar.NewInt("lsp_lease_detached")
	lspLeaseEvictedTotal  = expvar.NewInt("lsp_lease_evicted_total")
	lspLeaseReleasedTotal = expvar.NewMap("lsp_lease_released_total")
)
