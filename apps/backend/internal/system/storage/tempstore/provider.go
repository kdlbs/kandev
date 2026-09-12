// Package tempstore measures the server-selected system temporary folders.
package tempstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/system/storage/filescan"
)

const (
	defaultScanDeadline = 60 * time.Second
	windowsGOOS         = "windows"
)

type Status string

const (
	StatusMeasured      Status = "measured"
	StatusPartial       Status = "partial"
	StatusUnavailable   Status = "unavailable"
	StatusNotApplicable Status = "not_applicable"
)

type RootCandidate struct {
	RequestedPath string
	Optional      bool
}

type MountReader interface {
	Identity(string) (string, error)
}

type Scanner interface {
	MeasureWithOptions(
		context.Context,
		[]filescan.Root,
		filescan.MeasureOptions,
		func(filescan.Progress),
	) []filescan.Result
}

type Config struct {
	EffectiveRoot string
	UnixRoot      string
	GOOS          string
	RootResolver  func(context.Context) ([]RootCandidate, error)
	Mounts        MountReader
	Scanner       Scanner
	Deadline      time.Duration
	OnProgress    func(filescan.Progress)
}

type RootMeasurement struct {
	RequestedPath string   `json:"requested_path"`
	Path          string   `json:"path"`
	Aliases       []string `json:"aliases,omitempty"`
	Status        Status   `json:"status"`
	SizeBytes     *int64   `json:"size_bytes,omitempty"`
	SkippedCount  int      `json:"skipped_count,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type Analysis struct {
	Status          Status            `json:"status"`
	Roots           []RootMeasurement `json:"roots"`
	SizeBytes       *int64            `json:"size_bytes,omitempty"`
	IncludedInTotal bool              `json:"included_in_total"`
	Reason          string            `json:"reason,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
}

type Provider struct {
	effectiveRoot string
	unixRoot      string
	goos          string
	rootResolver  func(context.Context) ([]RootCandidate, error)
	mounts        MountReader
	scanner       Scanner
	deadline      time.Duration
	onProgress    func(filescan.Progress)
}

func New(config Config) *Provider {
	goos := config.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	effectiveRoot := config.EffectiveRoot
	if effectiveRoot == "" {
		effectiveRoot = os.TempDir()
	}
	unixRoot := config.UnixRoot
	if unixRoot == "" {
		unixRoot = "/tmp"
	}
	mounts := config.Mounts
	if mounts == nil {
		mounts = newMountReader()
	}
	scanner := config.Scanner
	if scanner == nil {
		scanner = filescan.NewLimiter(4)
	}
	deadline := config.Deadline
	if deadline <= 0 {
		deadline = defaultScanDeadline
	}
	resolver := config.RootResolver
	if resolver == nil {
		resolver = func(context.Context) ([]RootCandidate, error) {
			candidates := []RootCandidate{{RequestedPath: effectiveRoot}}
			if goos != windowsGOOS && unixRoot != "" {
				candidates = append(candidates, RootCandidate{RequestedPath: unixRoot, Optional: true})
			}
			return candidates, nil
		}
	}
	return &Provider{
		effectiveRoot: effectiveRoot, unixRoot: unixRoot, goos: goos,
		rootResolver: resolver, mounts: mounts, scanner: scanner,
		deadline: deadline, onProgress: config.OnProgress,
	}
}

func NewProvider(config Config) *Provider { return New(config) }

func (p *Provider) AnalyzeWithProgress(
	ctx context.Context,
	onProgress func(filescan.Progress),
) (Analysis, error) {
	copy := *p
	copy.onProgress = onProgress
	return copy.Analyze(ctx)
}

func (p *Provider) Analyze(ctx context.Context) (Analysis, error) {
	if err := ctx.Err(); err != nil {
		return Analysis{}, err
	}
	candidates, err := p.resolveRoots(ctx)
	if err != nil {
		return Analysis{
			Status: StatusUnavailable, IncludedInTotal: false,
			Reason: "root_resolution_failed", Warnings: []string{err.Error()},
		}, err
	}
	plans := p.planRoots(candidates)
	analysis := Analysis{IncludedInTotal: false, Roots: make([]RootMeasurement, len(plans))}
	scanRoots := make([]filescan.Root, 0, len(plans))
	scanPlans := make([]int, 0, len(plans))
	for index := range plans {
		analysis.Roots[index] = plans[index].measurement
		if plans[index].scan == nil {
			continue
		}
		scanPlans = append(scanPlans, index)
		scanRoots = append(scanRoots, *plans[index].scan)
	}
	if len(scanRoots) > 0 {
		scanCtx, cancel := context.WithTimeout(ctx, p.deadline)
		results := p.scanner.MeasureWithOptions(
			scanCtx, scanRoots,
			filescan.MeasureOptions{TolerateEntryErrors: true, CountSkipped: true, MaxWarnings: 10},
			p.onProgress,
		)
		scanDeadline := errors.Is(scanCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err := ctx.Err(); err != nil {
			return analysis, err
		}
		if err := scanCtx.Err(); err != nil && !errors.Is(err, context.Canceled) && !scanDeadline {
			return analysis, err
		}
		for scanIndex, planIndex := range scanPlans {
			result := filescan.Result{}
			if scanIndex < len(results) {
				result = results[scanIndex]
			} else {
				result.Err = errors.New("temporary root measurement returned no result")
			}
			analysis.Roots[planIndex] = measurementFromResult(
				analysis.Roots[planIndex], result, scanDeadline,
			)
		}
	}
	return summarize(analysis), nil
}

func (p *Provider) resolveRoots(ctx context.Context) ([]RootCandidate, error) {
	if p == nil || p.rootResolver == nil {
		return nil, errors.New("temporary root resolver is unavailable")
	}
	return p.rootResolver(ctx)
}

type rootPlan struct {
	measurement RootMeasurement
	scan        *filescan.Root
	canonical   string
	identity    string
}

func (p *Provider) planRoots(candidates []RootCandidate) []rootPlan {
	plans := make([]rootPlan, 0, len(candidates))
	for _, candidate := range candidates {
		requested, err := absoluteClean(candidate.RequestedPath)
		if err != nil {
			plans = append(plans, unavailablePlan(candidate.RequestedPath, candidate.Optional, err))
			continue
		}
		_, err = os.Lstat(requested)
		if errors.Is(err, os.ErrNotExist) {
			if candidate.Optional {
				plans = append(plans, rootPlan{measurement: RootMeasurement{
					RequestedPath: requested, Path: requested, Status: StatusNotApplicable,
					Reason: "root_not_present",
				}})
			} else {
				plans = append(plans, unavailablePlan(requested, false, errors.New("root does not exist")))
			}
			continue
		}
		if err != nil {
			plans = append(plans, unavailablePlan(requested, candidate.Optional, err))
			continue
		}
		canonical, err := filepath.EvalSymlinks(requested)
		if err != nil {
			plans = append(plans, unavailablePlan(requested, candidate.Optional, err))
			continue
		}
		canonical, err = absoluteClean(canonical)
		if err != nil {
			plans = append(plans, unavailablePlan(requested, candidate.Optional, err))
			continue
		}
		targetInfo, err := os.Stat(canonical)
		if err != nil {
			plans = append(plans, unavailablePlan(requested, candidate.Optional, err))
			continue
		}
		if !targetInfo.IsDir() {
			plans = append(plans, unavailablePlan(requested, candidate.Optional, errors.New("root is not a directory")))
			continue
		}
		if isFilesystemRoot(canonical) {
			plans = append(plans, unavailablePlanWithReason(requested, canonical, "unsafe_root"))
			continue
		}
		identity, err := p.mounts.Identity(canonical)
		if err != nil {
			plans = append(plans, unavailablePlan(requested, candidate.Optional, err))
			continue
		}
		guard := mountGuard{path: canonical, identity: identity, mounts: p.mounts, windows: p.goos == windowsGOOS}
		measurement := RootMeasurement{
			RequestedPath: requested, Path: canonical, Status: StatusMeasured,
		}
		scan := &filescan.Root{
			Path: canonical, SymlinkPolicy: filescan.SkipSymlinks,
			ShouldSkip: guard.shouldSkip, Validate: guard.validate,
		}
		plans = addRootPlan(plans, rootPlan{
			measurement: measurement, scan: scan, canonical: canonical, identity: identity,
		}, p.goos == windowsGOOS)
	}
	return plans
}

func absoluteClean(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("temporary root path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func unavailablePlan(requested string, optional bool, err error) rootPlan {
	if optional && errors.Is(err, os.ErrNotExist) {
		return rootPlan{measurement: RootMeasurement{
			RequestedPath: requested, Path: requested, Status: StatusNotApplicable,
			Reason: "root_not_present",
		}}
	}
	return rootPlan{measurement: RootMeasurement{
		RequestedPath: requested, Path: requested, Status: StatusUnavailable,
		Reason: "measurement_unavailable", Warnings: []string{err.Error()},
	}}
}

func unavailablePlanWithReason(requested, path, reason string) rootPlan {
	return rootPlan{measurement: RootMeasurement{
		RequestedPath: requested, Path: path, Status: StatusUnavailable, Reason: reason,
	}}
}

func addRootPlan(plans []rootPlan, candidate rootPlan, windows bool) []rootPlan {
	for index := range plans {
		existing := &plans[index]
		if existing.canonical == "" {
			continue
		}
		if samePath(existing.canonical, candidate.canonical, windows) {
			existing.measurement.Aliases = append(existing.measurement.Aliases, candidate.measurement.RequestedPath)
			return plans
		}
		if !sameMount(existing.identity, candidate.identity, windows) {
			continue
		}
		if pathWithin(existing.canonical, candidate.canonical) {
			existing.measurement.Aliases = append(existing.measurement.Aliases, candidate.measurement.RequestedPath)
			return plans
		}
	}

	nested := make([]int, 0)
	for index := range plans {
		existing := plans[index]
		if existing.canonical != "" &&
			sameMount(existing.identity, candidate.identity, windows) &&
			pathWithin(candidate.canonical, existing.canonical) {
			nested = append(nested, index)
		}
	}
	if len(nested) == 0 {
		return append(plans, candidate)
	}

	aliases := append([]string(nil), candidate.measurement.Aliases...)
	for _, index := range nested {
		existing := plans[index].measurement
		aliases = append(aliases, existing.RequestedPath)
		aliases = append(aliases, existing.Aliases...)
	}
	candidate.measurement.Aliases = aliases
	first := nested[0]
	result := make([]rootPlan, 0, len(plans)-len(nested)+1)
	for index, plan := range plans {
		if index == first {
			result = append(result, candidate)
		}
		if containsIndex(nested, index) {
			continue
		}
		result = append(result, plan)
	}
	return result
}

func containsIndex(indices []int, target int) bool {
	for _, index := range indices {
		if index == target {
			return true
		}
	}
	return false
}

type mountGuard struct {
	path     string
	identity string
	mounts   MountReader
	windows  bool
}

func (g mountGuard) validate() error {
	canonical, err := filepath.EvalSymlinks(g.path)
	if err != nil {
		return fmt.Errorf("temporary root changed: %w", err)
	}
	canonical, err = absoluteClean(canonical)
	if err != nil || !samePath(canonical, g.path, g.windows) {
		return fmt.Errorf("temporary root was replaced: %s", g.path)
	}
	identity, err := g.mounts.Identity(g.path)
	if err != nil {
		return fmt.Errorf("inspect temporary root mount: %w", err)
	}
	if !sameMount(identity, g.identity, g.windows) {
		return fmt.Errorf("temporary root mount changed: %s", g.path)
	}
	return nil
}

func (g mountGuard) shouldSkip(path string, entry fs.DirEntry) (bool, error) {
	rootIdentity, err := g.mounts.Identity(g.path)
	if err != nil {
		return false, fmt.Errorf("inspect temporary root mount %s: %w", g.path, err)
	}
	if !sameMount(rootIdentity, g.identity, g.windows) {
		return false, fmt.Errorf("temporary root mount changed: %s", g.path)
	}
	if entry != nil && entry.Type()&os.ModeSymlink != 0 {
		return false, nil
	}
	identity, err := g.mounts.Identity(path)
	if err != nil {
		return false, fmt.Errorf("inspect temporary path mount %s: %w", path, err)
	}
	return !sameMount(identity, g.identity, g.windows), nil
}

func measurementFromResult(
	measurement RootMeasurement,
	result filescan.Result,
	deadline bool,
) RootMeasurement {
	measurement.SkippedCount = result.SkippedCount
	measurement.Warnings = appendBounded(nil, result.Warnings, 10)
	if result.Err != nil {
		if deadline && errors.Is(result.Err, context.DeadlineExceeded) {
			size := result.Bytes
			measurement.Status = StatusPartial
			measurement.SizeBytes = &size
			measurement.Reason = "deadline"
			measurement.Warnings = appendBounded(
				measurement.Warnings, []string{result.Err.Error()}, 10,
			)
			return measurement
		}
		measurement.Status = StatusUnavailable
		measurement.SizeBytes = nil
		measurement.Reason = "measurement_unavailable"
		measurement.Warnings = appendBounded(measurement.Warnings, []string{result.Err.Error()}, 10)
		return measurement
	}
	size := result.Bytes
	measurement.SizeBytes = &size
	if result.Partial {
		measurement.Status = StatusPartial
		measurement.Reason = "some_entries_unmeasured"
	}
	return measurement
}

func summarize(analysis Analysis) Analysis {
	var measured, partial, unavailable, notApplicable int
	var total int64
	for index := range analysis.Roots {
		root := &analysis.Roots[index]
		switch root.Status {
		case StatusMeasured:
			measured++
		case StatusPartial:
			partial++
		case StatusUnavailable:
			unavailable++
		case StatusNotApplicable:
			notApplicable++
		}
		if root.SizeBytes != nil {
			total += *root.SizeBytes
		}
		analysis.Warnings = appendBounded(analysis.Warnings, root.Warnings, 10)
	}
	switch {
	case measured+partial > 0:
		size := total
		analysis.SizeBytes = &size
		analysis.Reason = "informational_overlap"
		for _, root := range analysis.Roots {
			if root.Reason == "deadline" {
				analysis.Reason = "deadline"
				break
			}
		}
		if partial > 0 || unavailable > 0 {
			analysis.Status = StatusPartial
		} else {
			analysis.Status = StatusMeasured
		}
	case unavailable > 0:
		analysis.Status = StatusUnavailable
		analysis.Reason = "measurement_unavailable"
	case notApplicable > 0:
		analysis.Status = StatusNotApplicable
		analysis.Reason = "root_not_present"
	default:
		analysis.Status = StatusNotApplicable
		analysis.Reason = "no_selected_roots"
	}
	return analysis
}

func appendBounded(values, additions []string, limit int) []string {
	for _, addition := range additions {
		if len(values) >= limit {
			break
		}
		if addition != "" {
			values = append(values, addition)
		}
	}
	return values
}

func sameMount(left, right string, windows bool) bool {
	if windows {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func samePath(left, right string, windows bool) bool {
	if windows {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	return clean == volume+string(filepath.Separator)
}

func pathWithin(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
