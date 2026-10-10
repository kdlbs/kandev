package process

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"go.uber.org/zap"
)

func BenchmarkTurnCheckpoint(b *testing.B) {
	for _, trackedFiles := range []int{100, 20_000} {
		b.Run(fmt.Sprintf("%dTrackedFiles", trackedFiles), func(b *testing.B) {
			repo := benchmarkTurnChangeRepository(b, trackedFiles, nil)
			benchmarkTurnChangeInterval(b, repo, "tracked-000000.txt", []byte("changed tracked content\n"))
		})
	}
}

func BenchmarkTurnChangeExportLargeTextAndBinary(b *testing.B) {
	b.Run("1MiBTextAndBinary", func(b *testing.B) {
		line := []byte("turn checkpoint benchmark text line\n")
		text := bytes.Repeat(line, (1<<20)/len(line))
		binary := make([]byte, 1<<20)
		if _, err := rand.New(rand.NewSource(42)).Read(binary); err != nil {
			b.Fatal(err)
		}
		repo := benchmarkTurnChangeRepository(b, 0, map[string][]byte{
			"large.txt": text,
			"large.bin": binary,
		})
		updatedText := append(bytes.Clone(text), []byte("changed after admission\n")...)
		updatedBinary := bytes.Clone(binary)
		updatedBinary[len(updatedBinary)/2] ^= 0xff
		benchmarkTurnChangeFiles(b, repo, map[string][]byte{
			"large.txt": updatedText,
			"large.bin": updatedBinary,
		})
	})
}

func BenchmarkTurnCheckpointCommittedChanges(b *testing.B) {
	repo := benchmarkTurnChangeRepository(b, 100, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		started := time.Now()
		exportBytes, err := runCommittedTurnChangeInterval(
			b,
			repo,
			"tracked-000000.txt",
			[]byte(fmt.Sprintf("committed turn %d\n", index)),
		)
		if err != nil {
			b.Fatal(err)
		}
		b.Logf("observation_us=%d exported_bytes=%d", time.Since(started).Microseconds(), exportBytes)
	}
}

func BenchmarkTurnCheckpointTwoCheckouts(b *testing.B) {
	first := benchmarkTurnChangeRepository(b, 100, nil)
	second := benchmarkTurnChangeRepository(b, 100, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		started := time.Now()
		firstBytes, err := runTurnChangeInterval(b, first, "tracked-000000.txt", []byte(fmt.Sprintf("first checkout %d\n", index)))
		if err != nil {
			b.Fatal(err)
		}
		secondBytes, err := runTurnChangeInterval(b, second, "tracked-000000.txt", []byte(fmt.Sprintf("second checkout %d\n", index)))
		if err != nil {
			b.Fatal(err)
		}
		b.Logf("observation_us=%d exported_bytes=%d", time.Since(started).Microseconds(), firstBytes+secondBytes)
	}
	b.ReportMetric(2, "checkouts/op")
}

func BenchmarkTurnCheckpointSharedCheckoutContention(b *testing.B) {
	repo := benchmarkTurnChangeRepository(b, 100, nil)
	operator, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		b.Fatal(err)
	}
	operators := []*GitOperator{NewGitOperator(repo.root, operator, nil), NewGitOperator(repo.root, operator, nil)}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		started := time.Now()
		changeSetID := uuid.NewString()
		finished := make(chan error, len(operators))
		for _, gitOperator := range operators {
			go func(gitOperator *GitOperator) {
				_, captureErr := gitOperator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
					ChangeSetID: changeSetID, CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
				})
				finished <- captureErr
			}(gitOperator)
		}
		for range operators {
			if captureErr := <-finished; captureErr != nil {
				b.Fatal(captureErr)
			}
		}
		b.Logf("observation_us=%d", time.Since(started).Microseconds())
	}
	b.ReportMetric(2, "concurrent-captures/op")
}

type turnChangeBenchmarkRepo struct {
	root     string
	operator *GitOperator
}

func benchmarkTurnChangeRepository(b *testing.B, trackedFiles int, extraFiles map[string][]byte) turnChangeBenchmarkRepo {
	b.Helper()
	root := b.TempDir()
	benchmarkGit(b, root, "init", "--quiet")
	benchmarkGit(b, root, "config", "user.name", "Kandev Benchmark")
	benchmarkGit(b, root, "config", "user.email", "benchmark@example.invalid")
	for index := 0; index < trackedFiles; index++ {
		name := fmt.Sprintf("tracked-%06d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte("tracked benchmark fixture\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	for name, content := range extraFiles {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	benchmarkGit(b, root, "add", "--all")
	benchmarkGit(b, root, "commit", "--quiet", "-m", "benchmark fixture")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		b.Fatal(err)
	}
	return turnChangeBenchmarkRepo{root: root, operator: NewGitOperator(root, log, nil)}
}

func benchmarkTurnChangeInterval(b *testing.B, repo turnChangeBenchmarkRepo, changedPath string, content []byte) {
	b.Helper()
	benchmarkTurnChangeFiles(b, repo, map[string][]byte{changedPath: content})
}

func benchmarkTurnChangeFiles(b *testing.B, repo turnChangeBenchmarkRepo, changedFiles map[string][]byte) {
	b.Helper()
	b.ReportAllocs()
	initialObjects, initialSize := benchmarkGitObjectTotals(b, repo.root)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		started := time.Now()
		observation := make(map[string][]byte, len(changedFiles))
		for path, content := range changedFiles {
			observation[path] = append(bytes.Clone(content), []byte(fmt.Sprintf("\nobservation %d\n", index))...)
		}
		exportBytes, err := runTurnChangeFilesInterval(b, repo, observation)
		if err != nil {
			b.Fatal(err)
		}
		b.Logf("observation_us=%d exported_bytes=%d", time.Since(started).Microseconds(), exportBytes)
	}
	b.StopTimer()
	finalObjects, finalSize := benchmarkGitObjectTotals(b, repo.root)
	b.ReportMetric(float64(finalObjects-initialObjects)/float64(b.N), "new-objects/op")
	b.ReportMetric(float64(finalSize-initialSize)/float64(b.N), "loose-KiB/op")
	var changedBytes int
	for _, content := range changedFiles {
		changedBytes += len(content)
	}
	b.ReportMetric(float64(changedBytes), "changed-bytes/op")
}

func runTurnChangeInterval(b *testing.B, repo turnChangeBenchmarkRepo, changedPath string, content []byte) (int64, error) {
	return runTurnChangeFilesInterval(b, repo, map[string][]byte{changedPath: content})
}

func runTurnChangeFilesInterval(b *testing.B, repo turnChangeBenchmarkRepo, changedFiles map[string][]byte) (int64, error) {
	b.Helper()
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	request := turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	}
	if _, err := repo.operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
		return 0, err
	}
	for path, content := range changedFiles {
		if err := os.WriteFile(filepath.Join(repo.root, path), content, 0o644); err != nil {
			return 0, err
		}
	}
	request.Boundary = turnchanges.CheckpointEnd
	if _, err := repo.operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
		return 0, err
	}
	pair := acceptedTurnCheckpointPair(b, repo.operator, changeSetID, checkoutID, "")
	comparison, err := repo.operator.CompareTurnCheckpoints(context.Background(), pair)
	if err != nil {
		return 0, err
	}
	if !comparison.Complete {
		return 0, fmt.Errorf("incomplete comparison: %s", comparison.Reason)
	}
	result, err := repo.operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(pair))
	if err != nil {
		return 0, err
	}
	return result.ExportBytes, nil
}

func runCommittedTurnChangeInterval(
	b *testing.B,
	repo turnChangeBenchmarkRepo,
	changedPath string,
	content []byte,
) (int64, error) {
	b.Helper()
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	request := turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	}
	if _, err := repo.operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(repo.root, changedPath), content, 0o644); err != nil {
		return 0, err
	}
	benchmarkGit(b, repo.root, "add", "--", changedPath)
	benchmarkGit(b, repo.root, "commit", "--quiet", "-m", "benchmark turn change")
	request.Boundary = turnchanges.CheckpointEnd
	if _, err := repo.operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
		return 0, err
	}
	result, err := repo.operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(
		acceptedTurnCheckpointPair(b, repo.operator, changeSetID, checkoutID, ""),
	))
	if err != nil {
		return 0, err
	}
	return result.ExportBytes, nil
}

func benchmarkGitObjectTotals(b *testing.B, repo string) (int64, int64) {
	b.Helper()
	output, err := exec.Command("git", "-C", repo, "count-objects", "-v").Output()
	if err != nil {
		b.Fatal(err)
	}
	var objects, size int64
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.SplitN(line, ": ", 2)
		if len(fields) != 2 {
			continue
		}
		value, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "count":
			objects = value
		case "size":
			size = value
		}
	}
	return objects, size
}

func benchmarkGit(b *testing.B, repo string, args ...string) {
	b.Helper()
	command := exec.Command("git", args...)
	command.Dir = repo
	if output, err := command.CombinedOutput(); err != nil {
		b.Fatalf("git %v: %v: %s", args, err, output)
	}
}
