package toolretention

func rowFitsBatch(used, size int64) bool { return used == 0 || size <= batchBytes-used }
