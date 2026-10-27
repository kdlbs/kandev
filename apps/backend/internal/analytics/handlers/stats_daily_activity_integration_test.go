package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/dto"
	analyticsqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type dailyActivityFixture struct {
	t      *testing.T
	repo   *tasksqlite.Repository
	router *gin.Engine
	today  time.Time
}

func newDailyActivityFixture(t *testing.T) *dailyActivityFixture {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	database := sqlx.NewDb(raw, "sqlite3")
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	analytics, err := analyticsqlite.NewWithDB(database, database)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"main", "other"} {
		if err := tasks.CreateWorkspace(context.Background(), &models.Workspace{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterStatsRoutes(router, analytics, newOwnerAuthorizer(), authzTestLogger(t))
	return &dailyActivityFixture{t: t, repo: tasks, router: router, today: time.Now().UTC().Truncate(24 * time.Hour)}
}

func (f *dailyActivityFixture) seed(id, workspace, origin string, ephemeral bool, turnAt, messageAt time.Time) {
	f.t.Helper()
	ctx := context.Background()
	task := &models.Task{ID: id, WorkspaceID: workspace, Title: id, Origin: origin, IsEphemeral: ephemeral}
	if err := f.repo.CreateTask(ctx, task); err != nil {
		f.t.Fatal(err)
	}
	if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{ID: id + "-session", TaskID: id, StartedAt: turnAt}); err != nil {
		f.t.Fatal(err)
	}
	f.turn(id, id+"-turn", turnAt)
	msg := &models.Message{ID: id + "-message", TaskID: id, TaskSessionID: id + "-session", TurnID: id + "-turn", AuthorType: models.MessageAuthorAgent, Content: "output", CreatedAt: messageAt}
	if err := f.repo.CreateMessage(ctx, msg); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dailyActivityFixture) turn(taskID, turnID string, at time.Time) {
	f.t.Helper()
	if err := f.repo.CreateTurn(context.Background(), &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: taskID + "-session", StartedAt: at}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dailyActivityFixture) read(workspace string) []dto.DailyActivityDTO {
	f.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace+"/stats/daily-activity?range=week", nil)
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		f.t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var result []dto.DailyActivityDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		f.t.Fatal(err)
	}
	if time.Now().UTC().Truncate(24*time.Hour) != f.today {
		f.t.Skip("UTC date changed during fixture")
	}
	if len(result) != 7 {
		f.t.Fatalf("got %d days, want 7", len(result))
	}
	for i, row := range result {
		want := f.today.AddDate(0, 0, i-6).Format("2006-01-02")
		if row.Date != want {
			f.t.Errorf("row %d date = %s, want %s", i, row.Date, want)
		}
	}
	return result
}

// @covers AC-TASKS-DAILY-ACTIVITY-001.1 through .4 and .6
func TestDailyActivityOvernightRegisteredRoute(t *testing.T) {
	for _, tc := range []struct {
		name        string
		turnDaysAgo int
	}{{"previous-day", 1}, {"before-horizon", 8}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDailyActivityFixture(t)
			f.seed("ongoing", "main", "", false, f.today.AddDate(0, 0, -tc.turnDaysAgo).Add(23*time.Hour), f.today.Add(time.Hour))
			beforeTask, err := f.repo.GetTask(context.Background(), "ongoing")
			if err != nil {
				t.Fatal(err)
			}
			beforeSession, err := f.repo.GetTaskSession(context.Background(), "ongoing-session")
			if err != nil {
				t.Fatal(err)
			}
			beforeTurn, err := f.repo.GetTurn(context.Background(), "ongoing-turn")
			if err != nil {
				t.Fatal(err)
			}
			before, err := f.repo.GetMessage(context.Background(), "ongoing-message")
			if err != nil {
				t.Fatal(err)
			}
			rows := f.read("main")
			want := dto.DailyActivityDTO{Date: f.today.Format("2006-01-02"), MessageCount: 1}
			if got := rows[6]; got != want {
				t.Errorf("overnight activity = %+v, want %+v", got, want)
			}
			after, err := f.repo.GetMessage(context.Background(), "ongoing-message")
			if err != nil {
				t.Fatal(err)
			}
			afterTask, err := f.repo.GetTask(context.Background(), "ongoing")
			if err != nil {
				t.Fatal(err)
			}
			afterSession, err := f.repo.GetTaskSession(context.Background(), "ongoing-session")
			if err != nil {
				t.Fatal(err)
			}
			afterTurn, err := f.repo.GetTurn(context.Background(), "ongoing-turn")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeTask, afterTask) || !reflect.DeepEqual(beforeSession, afterSession) || !reflect.DeepEqual(beforeTurn, afterTurn) {
				t.Error("reading statistics changed persisted activity")
			}
			f.turn("ongoing", "new-turn", f.today.Add(2*time.Hour))
			rows = f.read("main")
			want.TurnCount, want.TaskCount = 1, 1
			if got := rows[6]; got != want {
				t.Errorf("new turn activity = %+v, want %+v", got, want)
			}
		})
	}
	t.Run("controls-and-boundaries", func(t *testing.T) {
		f := newDailyActivityFixture(t)
		f.seed("same", "main", "", false, f.today.Add(time.Hour), f.today.Add(2*time.Hour))
		f.turn("same", "same-extra", f.today.Add(3*time.Hour))
		for _, message := range []*models.Message{
			{ID: "same-user", TaskID: "same", TaskSessionID: "same-session", TurnID: "same-turn", AuthorType: models.MessageAuthorUser, Content: "prompt", CreatedAt: f.today.Add(4 * time.Hour)},
			{ID: "same-tool", TaskID: "same", TaskSessionID: "same-session", TurnID: "same-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall, Content: "tool", CreatedAt: f.today.Add(5 * time.Hour)},
		} {
			if err := f.repo.CreateMessage(context.Background(), message); err != nil {
				t.Fatal(err)
			}
		}
		f.seed("start", "main", "", false, f.today.AddDate(0, 0, -6), f.today.AddDate(0, 0, -6))
		f.seed("outside", "main", "", false, f.today.AddDate(0, 0, -7), f.today.AddDate(0, 0, -7))
		f.seed("future", "main", "", false, f.today.AddDate(0, 0, 1), f.today.AddDate(0, 0, 1))
		f.seed("ephemeral", "main", "", true, f.today, f.today)
		f.seed("automation", "main", models.TaskOriginAutomationRun, false, f.today, f.today)
		f.seed("foreign", "other", "", false, f.today, f.today)
		rows := f.read("main")
		for i, row := range rows {
			want := dto.DailyActivityDTO{Date: f.today.AddDate(0, 0, i-6).Format("2006-01-02")}
			switch i {
			case 0:
				want.TurnCount, want.MessageCount, want.TaskCount = 1, 1, 1
			case 6:
				want.TurnCount, want.MessageCount, want.TaskCount = 2, 3, 1
			}
			if row != want {
				t.Errorf("day %d = %+v, want %+v", i, row, want)
			}
		}
		other := f.read("other")
		if got := other[6]; got.MessageCount != 1 || got.TurnCount != 1 || got.TaskCount != 1 {
			t.Errorf("foreign workspace own activity = %+v", got)
		}
	})
}
