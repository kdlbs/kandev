package startup

import (
	"math"
	"math/big"
	"strconv"
)

// byteUnitSymbols are display-only unit symbols. Every surface and every
// locale shows the same B/KB/MB/GB/TB symbols, so they are not localized.
var byteUnitSymbols = [...]string{"B", "KB", "MB", "GB", "TB"}

const bytesPerUnit = 1024

// FormatBytes renders an integer byte count with 1024-based units: whole B
// below 1 KiB, one fractional digit above it, the largest unit selected before
// rounding, and TB as the cap. Zero reads "0 B". This matches
// apps/web/lib/utils/format-bytes.ts; the numeric cases in bytes_test.go and
// format.test.ts are the parity contract.
func FormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	unit := 0
	value := float64(bytes)
	for unit < len(byteUnitSymbols)-1 && value >= bytesPerUnit {
		value /= bytesPerUnit
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	return scaledDecimal(value) + " " + byteUnitSymbols[unit]
}

// FormatByteRate renders a bytes-per-second rate with the same thresholds as
// FormatBytes plus a "/s" suffix. Rates keep one fractional digit even below
// 1 KiB, so a slow backup reads "0.5 B/s" rather than "0 B/s".
func FormatByteRate(rate float64) string {
	if !(rate > 0) || math.IsInf(rate, 0) {
		return "0.0 B/s"
	}
	unit := 0
	for unit < len(byteUnitSymbols)-1 && rate >= bytesPerUnit {
		rate /= bytesPerUnit
		unit++
	}
	return scaledDecimal(rate) + " " + byteUnitSymbols[unit] + "/s"
}

// scaledDecimal renders a value with one decimal digit exactly as JavaScript's
// Number.prototype.toFixed(1) does: the exact double is rounded to the nearest
// tenth, with an exact tie rounded up. Do not replace it with a
// multiply-then-round: that changes values just below a midpoint. The caller
// passes a positive finite value; both entry points guard their own zero,
// negative, and non-finite inputs.
func scaledDecimal(value float64) string {
	exact := new(big.Rat).SetFloat64(value)
	exact.Mul(exact, big.NewRat(10, 1))
	exact.Add(exact, big.NewRat(1, 2))
	tenths := new(big.Int).Quo(exact.Num(), exact.Denom())
	whole, frac := new(big.Int).QuoRem(tenths, big.NewInt(10), new(big.Int))
	return whole.String() + "." + frac.String()
}
