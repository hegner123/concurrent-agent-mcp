package db

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// newTestDB creates a temporary database for testing
func newTestDB(t *testing.T) *DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "hq-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	db, err := NewDB(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- SQLiteTime Tests ---

func TestSQLiteTime_Scan(t *testing.T) {
	t.Run("valid datetime", func(t *testing.T) {
		var st SQLiteTime
		err := st.Scan("2024-01-15 10:30:45")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st.Time.Year() != 2024 || st.Time.Month() != 1 || st.Time.Day() != 15 {
			t.Errorf("wrong date: %v", st.Time)
		}
		if st.Time.Hour() != 10 || st.Time.Minute() != 30 || st.Time.Second() != 45 {
			t.Errorf("wrong time: %v", st.Time)
		}
	})

	t.Run("nil value", func(t *testing.T) {
		var st SQLiteTime
		err := st.Scan(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !st.Time.IsZero() {
			t.Errorf("expected zero time, got: %v", st.Time)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		var st SQLiteTime
		err := st.Scan(12345)
		if err == nil {
			t.Fatal("expected error for invalid type")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		var st SQLiteTime
		err := st.Scan("not-a-date")
		if err == nil {
			t.Fatal("expected error for invalid format")
		}
	})
}

func TestNullSQLiteTime_Scan(t *testing.T) {
	t.Run("valid datetime", func(t *testing.T) {
		var nst NullSQLiteTime
		err := nst.Scan("2024-06-20 14:45:30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !nst.Valid {
			t.Error("expected Valid to be true")
		}
		if nst.Time.Year() != 2024 || nst.Time.Month() != 6 || nst.Time.Day() != 20 {
			t.Errorf("wrong date: %v", nst.Time)
		}
	})

	t.Run("nil value", func(t *testing.T) {
		var nst NullSQLiteTime
		err := nst.Scan(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nst.Valid {
			t.Error("expected Valid to be false")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		var nst NullSQLiteTime
		err := nst.Scan([]byte("test"))
		if err == nil {
			t.Fatal("expected error for invalid type")
		}
	})
}

func TestNullSQLiteTime_Value(t *testing.T) {
	t.Run("valid time", func(t *testing.T) {
		nst := NullSQLiteTime{
			Time:  time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC),
			Valid: true,
		}
		val, err := nst.Value()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "2024-03-15 09:30:00"
		if val != expected {
			t.Errorf("expected %q, got %q", expected, val)
		}
	})

	t.Run("null time", func(t *testing.T) {
		nst := NullSQLiteTime{Valid: false}
		val, err := nst.Value()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != nil {
			t.Errorf("expected nil, got %v", val)
		}
	})
}

// --- Project Operation Tests ---

func TestCreateProject(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{
		{StepNum: 1, Branch: "feature/auth", Scope: "backend"},
		{StepNum: 2, Branch: "feature/auth-ui", Scope: "frontend", DependsOn: []int{1}},
	}

	project, err := db.CreateProject("test-project", "abc123", steps)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	if project.Name != "test-project" {
		t.Errorf("expected name 'test-project', got %q", project.Name)
	}
	if !project.BaseCommit.Valid || project.BaseCommit.String != "abc123" {
		t.Errorf("expected base_commit 'abc123', got %v", project.BaseCommit)
	}
	if project.Status != "active" {
		t.Errorf("expected status 'active', got %q", project.Status)
	}
}

func TestCreateProject_DuplicateName(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}

	_, err := db.CreateProject("dup-project", "abc", steps)
	if err != nil {
		t.Fatalf("first CreateProject failed: %v", err)
	}

	_, err = db.CreateProject("dup-project", "def", steps)
	if err == nil {
		t.Fatal("expected error for duplicate project name")
	}
}

func TestGetProject(t *testing.T) {
	db := newTestDB(t)

	t.Run("existing project", func(t *testing.T) {
		steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
		_, err := db.CreateProject("get-test", "abc", steps)
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		project, err := db.GetProject("get-test")
		if err != nil {
			t.Fatalf("GetProject failed: %v", err)
		}
		if project.Name != "get-test" {
			t.Errorf("expected name 'get-test', got %q", project.Name)
		}
	})

	t.Run("non-existing project", func(t *testing.T) {
		_, err := db.GetProject("non-existent")
		if err != sql.ErrNoRows {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})
}

func TestListProjects(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}

	_, err := db.CreateProject("list-1", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateProject("list-2", "def", steps)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("no filter", func(t *testing.T) {
		projects, err := db.ListProjects(nil)
		if err != nil {
			t.Fatalf("ListProjects failed: %v", err)
		}
		if len(projects) != 2 {
			t.Errorf("expected 2 projects, got %d", len(projects))
		}
	})

	t.Run("with status filter", func(t *testing.T) {
		status := "active"
		projects, err := db.ListProjects(&status)
		if err != nil {
			t.Fatalf("ListProjects failed: %v", err)
		}
		if len(projects) != 2 {
			t.Errorf("expected 2 active projects, got %d", len(projects))
		}

		status = "completed"
		projects, err = db.ListProjects(&status)
		if err != nil {
			t.Fatalf("ListProjects failed: %v", err)
		}
		if len(projects) != 0 {
			t.Errorf("expected 0 completed projects, got %d", len(projects))
		}
	})
}

// --- Step Lifecycle Tests ---

func TestClaimStep(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{
		{StepNum: 1, Branch: "step-1", Scope: "backend"},
		{StepNum: 2, Branch: "step-2", Scope: "backend"},
	}
	_, err := db.CreateProject("claim-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("claim-test", "agent-1")
	if err != nil {
		t.Fatalf("ClaimStep failed: %v", err)
	}
	if step == nil {
		t.Fatal("expected step, got nil")
	}
	if step.StepNum != 1 {
		t.Errorf("expected step_num 1, got %d", step.StepNum)
	}
	if step.Status != "claimed" {
		t.Errorf("expected status 'claimed', got %q", step.Status)
	}
	if !step.AgentID.Valid || step.AgentID.String != "agent-1" {
		t.Errorf("expected agent_id 'agent-1', got %v", step.AgentID)
	}
}

func TestClaimStep_NoWorkAvailable(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("no-work", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	// Claim the only step
	_, err = db.ClaimStep("no-work", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	// Try to claim again - no work available
	step, err := db.ClaimStep("no-work", "agent-2")
	if err != nil {
		t.Fatalf("ClaimStep failed: %v", err)
	}
	if step != nil {
		t.Errorf("expected nil (no work available), got step %d", step.StepNum)
	}
}

func TestClaimStep_RespectsDependencies(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{
		{StepNum: 1, Branch: "step-1", Scope: "backend"},
		{StepNum: 2, Branch: "step-2", Scope: "backend", DependsOn: []int{1}},
	}
	_, err := db.CreateProject("dep-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	// Step 1 should be claimable
	step1, err := db.ClaimStep("dep-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if step1.StepNum != 1 {
		t.Errorf("expected step 1, got step %d", step1.StepNum)
	}

	// Step 2 should not be claimable yet (dependency not completed)
	step2, err := db.ClaimStep("dep-test", "agent-2")
	if err != nil {
		t.Fatal(err)
	}
	if step2 != nil {
		t.Errorf("expected nil (step 2 blocked by dependency), got step %d", step2.StepNum)
	}

	// Complete step 1
	err = db.StartStep(step1.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.CompleteStep(step1.ID, "commit123", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Now step 2 should be claimable
	step2, err = db.ClaimStep("dep-test", "agent-2")
	if err != nil {
		t.Fatal(err)
	}
	if step2 == nil {
		t.Fatal("expected step 2 to be claimable after dependency completed")
	}
	if step2.StepNum != 2 {
		t.Errorf("expected step 2, got step %d", step2.StepNum)
	}
}

func TestClaimStep_Concurrent(t *testing.T) {
	db := newTestDB(t)

	// Create project with 1 step
	steps := []StepInput{{StepNum: 1, Branch: "concurrent", Scope: "backend"}}
	_, err := db.CreateProject("concurrent-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var claimed int
	var errors int

	// 10 goroutines racing to claim the single step
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			step, err := db.ClaimStep("concurrent-test", fmt.Sprintf("agent-%d", id))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// SQLITE_BUSY is expected under high contention
				errors++
				return
			}
			if step != nil {
				claimed++
			}
		}(i)
	}

	wg.Wait()

	// At most 1 agent should successfully claim the step
	// (some may get SQLITE_BUSY errors, which is acceptable)
	if claimed > 1 {
		t.Errorf("expected at most 1 claim, got %d", claimed)
	}
	if claimed == 0 && errors == 0 {
		t.Error("expected at least one claim or error")
	}
}

func TestStartStep(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("start-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("start-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	worktree := "/path/to/worktree"
	err = db.StartStep(step.ID, &worktree)
	if err != nil {
		t.Fatalf("StartStep failed: %v", err)
	}

	// Verify step status
	updatedStep, err := db.GetStep(step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedStep.Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got %q", updatedStep.Status)
	}
	if !updatedStep.Worktree.Valid || updatedStep.Worktree.String != worktree {
		t.Errorf("expected worktree %q, got %v", worktree, updatedStep.Worktree)
	}
}

func TestStartStep_WrongState(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("wrong-state", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	// Try to start a step that hasn't been claimed
	// First get the step ID
	step, err := db.ClaimStep("wrong-state", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	// Start it
	err = db.StartStep(step.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Try to start again (already in_progress)
	err = db.StartStep(step.ID, nil)
	if err == nil {
		t.Fatal("expected error for starting step not in claimed state")
	}
}

func TestCompleteStep(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("complete-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("complete-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	err = db.StartStep(step.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	files := []string{"src/main.go", "src/util.go"}
	notes := "Implemented feature"
	err = db.CompleteStep(step.ID, "def456", files, &notes)
	if err != nil {
		t.Fatalf("CompleteStep failed: %v", err)
	}

	// Verify
	updatedStep, err := db.GetStep(step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedStep.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", updatedStep.Status)
	}
	if !updatedStep.LastCommit.Valid || updatedStep.LastCommit.String != "def456" {
		t.Errorf("expected last_commit 'def456', got %v", updatedStep.LastCommit)
	}
	if !updatedStep.Notes.Valid || updatedStep.Notes.String != notes {
		t.Errorf("expected notes %q, got %v", notes, updatedStep.Notes)
	}
}

func TestCompleteStep_WrongState(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("complete-wrong", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("complete-wrong", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	// Try to complete without starting
	err = db.CompleteStep(step.ID, "abc", nil, nil)
	if err == nil {
		t.Fatal("expected error for completing step not in progress")
	}
}

func TestFailStep(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("fail-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("fail-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	err = db.StartStep(step.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = db.FailStep(step.ID, "test failed")
	if err != nil {
		t.Fatalf("FailStep failed: %v", err)
	}

	// Verify
	updatedStep, err := db.GetStep(step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedStep.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", updatedStep.Status)
	}
	if !updatedStep.Notes.Valid || updatedStep.Notes.String != "test failed" {
		t.Errorf("expected notes 'test failed', got %v", updatedStep.Notes)
	}
}

// --- Heartbeat Tests ---

func TestHeartbeat(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("heartbeat-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("heartbeat-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	// Heartbeat should succeed without error
	err = db.Heartbeat(step.ID, "agent-1")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Verify step still has valid heartbeat
	updatedStep, err := db.GetStep(step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updatedStep.LastHeartbeat.Valid {
		t.Error("heartbeat should be valid")
	}
}

func TestHeartbeat_WrongAgent(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("heartbeat-wrong", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("heartbeat-wrong", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	// Try heartbeat from wrong agent
	err = db.Heartbeat(step.ID, "agent-2")
	if err == nil {
		t.Fatal("expected error for heartbeat from wrong agent")
	}
}

func TestHeartbeat_WrongState(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("heartbeat-state", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("heartbeat-state", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	err = db.StartStep(step.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.CompleteStep(step.ID, "abc", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Try heartbeat on completed step
	err = db.Heartbeat(step.ID, "agent-1")
	if err == nil {
		t.Fatal("expected error for heartbeat on completed step")
	}
}

// --- Recovery Tests ---

func TestDetectStaleWork(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("stale-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("stale-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	// Manually set heartbeat to 20 minutes ago to make it stale
	_, err = db.conn.Exec(`
		UPDATE steps SET last_heartbeat = datetime('now', '-20 minutes')
		WHERE id = ?
	`, step.ID)
	if err != nil {
		t.Fatal(err)
	}

	// With default timeout (15 min), the step should be detected as stale
	staleSteps, err := db.DetectStaleWork(15)
	if err != nil {
		t.Fatalf("DetectStaleWork failed: %v", err)
	}

	found := false
	for _, s := range staleSteps {
		if s.ID == step.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected step to be detected as stale work")
	}
}

func TestRecoverStep(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("recover-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("recover-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	err = db.StartStep(step.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Recover the step
	err = db.RecoverStep(step.ID)
	if err != nil {
		t.Fatalf("RecoverStep failed: %v", err)
	}

	// Verify step is reset
	recoveredStep, err := db.GetStep(step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredStep.Status != "not_started" {
		t.Errorf("expected status 'not_started', got %q", recoveredStep.Status)
	}
	if recoveredStep.AgentID.Valid {
		t.Errorf("expected agent_id to be null, got %v", recoveredStep.AgentID)
	}
}

func TestRecoverFailedStep(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("recover-failed", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("recover-failed", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	err = db.StartStep(step.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.FailStep(step.ID, "original failure")
	if err != nil {
		t.Fatal(err)
	}

	// Recover the failed step
	err = db.RecoverFailedStep(step.ID)
	if err != nil {
		t.Fatalf("RecoverFailedStep failed: %v", err)
	}

	// Verify step is reset
	recoveredStep, err := db.GetStep(step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredStep.Status != "not_started" {
		t.Errorf("expected status 'not_started', got %q", recoveredStep.Status)
	}
	if recoveredStep.Notes.Valid {
		t.Errorf("expected notes to be null, got %v", recoveredStep.Notes)
	}

	// Verify history was recorded
	var count int
	err = db.conn.QueryRow(`
		SELECT COUNT(*) FROM failed_steps_history WHERE original_step_id = ?
	`, step.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 history record, got %d", count)
	}
}

// --- Metrics & Events Tests ---

func TestGetMetrics(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{
		{StepNum: 1, Branch: "step-1", Scope: "backend"},
		{StepNum: 2, Branch: "step-2", Scope: "frontend"},
		{StepNum: 3, Branch: "step-3", Scope: "backend"},
	}
	_, err := db.CreateProject("metrics-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	// Complete one step
	step1, err := db.ClaimStep("metrics-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.StartStep(step1.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteStep(step1.ID, "abc", nil, nil); err != nil {
		t.Fatal(err)
	}

	// Start another
	step2, err := db.ClaimStep("metrics-test", "agent-2")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.StartStep(step2.ID, nil); err != nil {
		t.Fatal(err)
	}

	projectName := "metrics-test"
	metrics, err := db.GetMetrics(&projectName, nil)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.TotalSteps != 3 {
		t.Errorf("expected 3 total steps, got %d", metrics.TotalSteps)
	}
	if metrics.CompletedSteps != 1 {
		t.Errorf("expected 1 completed step, got %d", metrics.CompletedSteps)
	}
	if metrics.InProgressSteps != 1 {
		t.Errorf("expected 1 in-progress step, got %d", metrics.InProgressSteps)
	}

	// Check scope breakdown
	if metrics.ScopeBreakdown["backend"] != 2 {
		t.Errorf("expected 2 backend steps, got %d", metrics.ScopeBreakdown["backend"])
	}
	if metrics.ScopeBreakdown["frontend"] != 1 {
		t.Errorf("expected 1 frontend step, got %d", metrics.ScopeBreakdown["frontend"])
	}
}

func TestGetAgentEvents(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{{StepNum: 1, Branch: "b", Scope: "s"}}
	_, err := db.CreateProject("events-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	step, err := db.ClaimStep("events-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.StartStep(step.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Heartbeat(step.ID, "agent-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteStep(step.ID, "abc", nil, nil); err != nil {
		t.Fatal(err)
	}

	agentID := "agent-1"
	events, err := db.GetAgentEvents(&agentID, nil, 10)
	if err != nil {
		t.Fatalf("GetAgentEvents failed: %v", err)
	}

	// Should have: claimed, started, heartbeat, completed
	if len(events) < 4 {
		t.Errorf("expected at least 4 events, got %d", len(events))
	}

	// Check that all expected event types are present
	eventTypeCounts := make(map[EventType]int)
	for _, e := range events {
		eventTypeCounts[e.EventType]++
	}

	expectedTypes := []EventType{EventTypeClaimed, EventTypeStarted, EventTypeHeartbeat, EventTypeCompleted}
	for _, expected := range expectedTypes {
		if eventTypeCounts[expected] == 0 {
			t.Errorf("expected event type %q not found", expected)
		}
	}
}

// --- GetAvailableSteps Tests ---

func TestGetAvailableSteps(t *testing.T) {
	db := newTestDB(t)

	steps := []StepInput{
		{StepNum: 1, Branch: "step-1", Scope: "backend"},
		{StepNum: 2, Branch: "step-2", Scope: "frontend"},
		{StepNum: 3, Branch: "step-3", Scope: "backend", DependsOn: []int{1}},
	}
	_, err := db.CreateProject("available-test", "abc", steps)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all available", func(t *testing.T) {
		projectName := "available-test"
		available, err := db.GetAvailableSteps(&projectName, nil)
		if err != nil {
			t.Fatalf("GetAvailableSteps failed: %v", err)
		}
		// Step 3 is blocked by dependency, so only 1 and 2 are available
		if len(available) != 2 {
			t.Errorf("expected 2 available steps, got %d", len(available))
		}
	})

	t.Run("filter by scope", func(t *testing.T) {
		projectName := "available-test"
		scope := "backend"
		available, err := db.GetAvailableSteps(&projectName, &scope)
		if err != nil {
			t.Fatalf("GetAvailableSteps failed: %v", err)
		}
		// Only step 1 is backend and available (step 3 is blocked)
		if len(available) != 1 {
			t.Errorf("expected 1 available backend step, got %d", len(available))
		}
	})
}
