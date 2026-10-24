package themes

import (
	"fmt"
	"math"
	"strconv"
)

// ContrastFindings reports supplied opaque foreground/background pairs without changing authored colors.
func ContrastFindings(d *Definition) []Finding {
	var findings []Finding
	for _, group := range []struct {
		name   string
		colors map[string]string
	}{{"ui", d.UI}, {"editor", d.Editor}, {"terminal", d.Terminal}} {
		foreground, ok := luminance(group.colors["foreground"])
		if !ok {
			continue
		}
		background, ok := luminance(group.colors["background"])
		if !ok {
			continue
		}
		ratio := (math.Max(foreground, background) + 0.05) / (math.Min(foreground, background) + 0.05)
		if ratio < 4.5 {
			findings = append(findings, Finding{Kind: "contrast", Field: group.name, Message: fmt.Sprintf("Foreground/background contrast %.2f:1 is below 4.5:1", ratio)})
		}
	}
	return findings
}
func luminance(color string) (float64, bool) {
	if len(color) != 7 {
		return 0, false
	}
	value, err := strconv.ParseUint(color[1:], 16, 24)
	if err != nil {
		return 0, false
	}
	components := []float64{float64(value>>16) / 255, float64((value>>8)&255) / 255, float64(value&255) / 255}
	for i, c := range components {
		if c <= 0.04045 {
			components[i] = c / 12.92
		} else {
			components[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*components[0] + 0.7152*components[1] + 0.0722*components[2], true
}
