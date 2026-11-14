package usagehistory

import "errors"

var ErrNotVisible = errors.New("usage target not found")
var ErrInvalidBatch = errors.New("invalid session usage batch")
