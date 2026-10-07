//go:build linux && cgo

package sqlite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil/sqlitememory"
)

const sidebarMemoryChild = "KANDEV_SIDEBAR_MEMORY_CHILD"
const sidebarMemoryCase = "KANDEV_SIDEBAR_MEMORY_CASE"
const sidebarMiB = int64(1024 * 1024)

func TestSidebarQueryPreparationMemory(t *testing.T) {
	if os.Getenv(sidebarMemoryChild) == "" {
		for _, count := range []int{0, 101} {
			t.Run(strconv.Itoa(count), func(t *testing.T) { runSidebarMemoryChild(t, "TestSidebarQueryPreparationMemory", count) })
		}
		return
	}
	repo, count := sidebarMemoryFixture(t)
	queries := sidebarMemoryQueries()
	for _, query := range queries {
		baseline := sqlitememory.Used()
		sqlitememory.Peak(true)
		started := time.Now()
		_, err := repo.QuerySidebarTaskPage(t.Context(), "memory-a", query, models.SidebarTaskViewPreferences{})
		if err != nil {
			t.Fatal(err)
		}
		peak := sqlitememory.Peak(false) - baseline
		t.Logf("tasks=%d sort=%s direction=%s group=%s native_peak_delta_bytes=%d retained_delta_bytes=%d rss_bytes=%d elapsed=%s",
			count, query.Sort.Key, query.Sort.Direction, query.Group, peak, sqlitememory.Used()-baseline, sidebarProcessRSS(t), time.Since(started))
		if peak > 64*sidebarMiB {
			t.Fatalf("native SQLite preparation peak %d bytes exceeds 64 MiB", peak)
		}
	}
}

func TestSidebarQueryPoolMemoryPlateau(t *testing.T) {
	if os.Getenv(sidebarMemoryChild) == "" {
		if os.Getenv("KANDEV_SIDEBAR_MEMORY_MATRIX") == "1" {
			for index, query := range sidebarMemoryQueries() {
				name := fmt.Sprintf("%s/%s/%s", query.Sort.Key, query.Group, query.Sort.Direction)
				t.Run(name, func(t *testing.T) {
					t.Setenv(sidebarMemoryCase, strconv.Itoa(index))
					runSidebarMemoryChild(t, "TestSidebarQueryPoolMemoryPlateau", 101)
				})
			}
		}
		t.Run("alternating", func(t *testing.T) {
			t.Setenv(sidebarMemoryCase, "")
			runSidebarMemoryChild(t, "TestSidebarQueryPoolMemoryPlateau", 101)
		})
		return
	}
	if got := runtime.GOMAXPROCS(0); got != 4 {
		t.Fatalf("pooled memory measurement must run with GOMAXPROCS=4, got %d", got)
	}
	repo, _ := sidebarMemoryFixture(t)
	queries := sidebarMemoryQueries()
	if selected := os.Getenv(sidebarMemoryCase); selected != "" {
		index, err := strconv.Atoi(selected)
		if err != nil {
			t.Fatal(err)
		}
		queries = queries[index : index+1]
	}
	// Warm every shape before retained-allocation measurements, on the production pool.
	for _, query := range queries {
		if _, err := repo.QuerySidebarTaskPage(t.Context(), "memory-a", query, models.SidebarTaskViewPreferences{}); err != nil {
			t.Fatal(err)
		}
	}
	// Warm all readers concurrently before measuring retained pool allocation.
	var warm sync.WaitGroup
	for range 4 {
		warm.Go(func() {
			if _, err := repo.QuerySidebarTaskPage(t.Context(), "memory-a", queries[0], models.SidebarTaskViewPreferences{}); err != nil {
				t.Error(err)
			}
		})
	}
	warm.Wait()
	if t.Failed() {
		return
	}
	started := time.Now()
	baseline, baselineRSS := sqlitememory.Used(), sidebarProcessRSS(t)
	sqlitememory.Peak(true)
	for start := 0; start < 100; start += 4 {
		var wg sync.WaitGroup
		errors := make(chan error, 4)
		for index := start; index < start+4; index++ {
			wg.Go(func() {
				workspaceID := "memory-a"
				if len(queries) > 1 && index%2 == 1 {
					workspaceID = "memory-b"
				}
				query := queries[index%len(queries)]
				query.Page = index%3 + 1
				if index%5 == 0 {
					query.CollapsedGroupKeys = []string{"__not_started__", "__unassigned__"}
				}
				result, err := repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, models.SidebarTaskViewPreferences{})
				if err == nil {
					for _, task := range result.Tasks {
						if task.WorkspaceID != workspaceID {
							err = fmt.Errorf("scratch data crossed workspaces")
						}
					}
				}
				errors <- err
			})
		}
		wg.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
		peak, retained, rss := sqlitememory.Peak(false)-baseline, sqlitememory.Used()-baseline, sidebarProcessRSS(t)-baselineRSS
		t.Logf("case=%s reads=%d concurrency=4 native_peak_delta_bytes=%d retained_delta_bytes=%d rss_delta_bytes=%d elapsed=%s", os.Getenv(sidebarMemoryCase), start+4, peak, retained, rss, time.Since(started))
		if peak > 256*sidebarMiB || retained > 8*sidebarMiB || rss > 512*sidebarMiB {
			t.Fatalf("pooled memory budget exceeded: native_peak=%d retained=%d rss=%d bytes", peak, retained, rss)
		}
	}
}

func runSidebarMemoryChild(t *testing.T, testName string, count int) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^"+testName+"$", "-test.v")
	command.Env = sidebarMemoryChildEnvironment(testName, count)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatalf("isolated SQLite memory regression: %v", err)
	}
}

func sidebarMemoryChildEnvironment(testName string, count int) []string {
	environment := os.Environ()
	if testName == "TestSidebarQueryPoolMemoryPlateau" {
		bounded := environment[:0]
		for _, entry := range environment {
			if !strings.HasPrefix(entry, "GOMAXPROCS=") {
				bounded = append(bounded, entry)
			}
		}
		environment = bounded
		environment = append(environment, "GOMAXPROCS=4")
	}
	return append(environment, sidebarMemoryChild+"="+strconv.Itoa(count))
}

func sidebarMemoryFixture(t *testing.T) (*Repository, int) {
	t.Helper()
	count, err := strconv.Atoi(os.Getenv(sidebarMemoryChild))
	if err != nil {
		t.Fatal(err)
	}
	repo := newSidebarReaderPool(t)
	for _, workspace := range []string{"memory-a", "memory-b"} {
		seedWorkspace(t, repo, workspace)
		insertSidebarScaleTasks(t, repo, workspace, workspace, count)
	}
	// Force all four readers open before the initialized baseline is sampled.
	connections := make([]interface{ Close() error }, 0, 4)
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	for range 4 {
		conn, err := repo.ro.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
	}
	t.Logf("go=%s os=%s arch=%s sqlite=%s pid=%d gomaxprocs=%d tasks_per_workspace=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, sqlitememory.Version(), os.Getpid(), runtime.GOMAXPROCS(0), count)
	return repo, count
}

func sidebarMemoryQueries() []models.SidebarTaskViewQuery {
	queries := make([]models.SidebarTaskViewQuery, 0, 72)
	for _, key := range []string{"lastActivityAt", "runningFirstActivity", "state", "updatedAt", "createdAt", "title", "custom"} {
		for _, group := range []string{"state", "none", "workflow", "workflowStep", "repository", "executorType"} {
			for _, direction := range []string{"asc", "desc"} {
				query := sidebarTaskQuery(1)
				query.Sort = models.SidebarTaskViewSort{Key: key, Direction: direction}
				query.Group = group
				queries = append(queries, query)
			}
		}
	}
	return queries
}

func sidebarProcessRSS(t testing.TB) int64 {
	t.Helper()
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		t.Fatalf("invalid process memory statistics")
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return pages * int64(os.Getpagesize())
}
