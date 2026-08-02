package sqlitestore

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"aegis/observability"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHandlerPersistsAndFiltersCorrelatedLogs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:observability-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(db, slog.LevelDebug)
	if err != nil {
		t.Fatal(err)
	}
	logger := observability.New(handler, observability.NewRedactor("db-secret"))
	ctx := observability.WithScope(context.Background(), observability.Scope{TaskID: "task-1", IssueID: "issue-1", ExecutionID: "exec-1", Component: "worker"})
	logger.Info(ctx, "completed", slog.String("detail", "db-secret"))
	logs, err := handler.Query(context.Background(), Filter{TaskID: "task-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].IssueID != "issue-1" || logs[0].ExecutionID != "exec-1" || logs[0].Attributes["detail"] != "[REDACTED]" {
		t.Fatalf("logs=%+v", logs)
	}
	if _, err = handler.Query(context.Background(), Filter{TaskID: "other", From: timePtr(time.Now().Add(-time.Hour))}); err != nil {
		t.Fatal(err)
	}
}

func timePtr(value time.Time) *time.Time { return &value }
