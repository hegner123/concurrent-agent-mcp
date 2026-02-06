package db

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// --- Custom ID Types ---

// ProjectID is a typed identifier for projects
type ProjectID int64

// StepID is a typed identifier for steps
type StepID int64

// AgentEventID is a typed identifier for agent events
type AgentEventID int64

// NullProjectID represents a nullable ProjectID
type NullProjectID struct {
	ProjectID ProjectID
	Valid     bool
}

// Scan implements sql.Scanner
func (n *NullProjectID) Scan(value any) error {
	if value == nil {
		n.ProjectID, n.Valid = 0, false
		return nil
	}
	n.Valid = true
	switch v := value.(type) {
	case int64:
		n.ProjectID = ProjectID(v)
	case int:
		n.ProjectID = ProjectID(v)
	default:
		return fmt.Errorf("cannot scan %T into NullProjectID", value)
	}
	return nil
}

// Value implements driver.Valuer
func (n NullProjectID) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return int64(n.ProjectID), nil
}

// NullStepID represents a nullable StepID
type NullStepID struct {
	StepID StepID
	Valid  bool
}

// Scan implements sql.Scanner
func (n *NullStepID) Scan(value any) error {
	if value == nil {
		n.StepID, n.Valid = 0, false
		return nil
	}
	n.Valid = true
	switch v := value.(type) {
	case int64:
		n.StepID = StepID(v)
	case int:
		n.StepID = StepID(v)
	default:
		return fmt.Errorf("cannot scan %T into NullStepID", value)
	}
	return nil
}

// Value implements driver.Valuer
func (n NullStepID) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return int64(n.StepID), nil
}

// --- Status Enum Types ---

// ProjectStatus represents the status of a project
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusAborted   ProjectStatus = "aborted"
)

// Valid returns true if the status is a valid ProjectStatus
func (s ProjectStatus) Valid() bool {
	switch s {
	case ProjectStatusActive, ProjectStatusCompleted, ProjectStatusAborted:
		return true
	}
	return false
}

// StepStatus represents the status of a step
type StepStatus string

const (
	StepStatusNotStarted StepStatus = "not_started"
	StepStatusClaimed    StepStatus = "claimed"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusFailed     StepStatus = "failed"
	StepStatusMerged     StepStatus = "merged"
)

// Valid returns true if the status is a valid StepStatus
func (s StepStatus) Valid() bool {
	switch s {
	case StepStatusNotStarted, StepStatusClaimed, StepStatusInProgress,
		StepStatusCompleted, StepStatusFailed, StepStatusMerged:
		return true
	}
	return false
}

// IsActive returns true if the step is in an active state (claimed or in_progress)
func (s StepStatus) IsActive() bool {
	return s == StepStatusClaimed || s == StepStatusInProgress
}

// IsDone returns true if the step is in a terminal state (completed, failed, or merged)
func (s StepStatus) IsDone() bool {
	return s == StepStatusCompleted || s == StepStatusFailed || s == StepStatusMerged
}

// EventType represents the type of an agent event
type EventType string

const (
	EventTypeClaimed   EventType = "claimed"
	EventTypeStarted   EventType = "started"
	EventTypeHeartbeat EventType = "heartbeat"
	EventTypeCompleted EventType = "completed"
	EventTypeFailed    EventType = "failed"
	EventTypeRecovered EventType = "recovered"
)

// Valid returns true if the event type is valid
func (e EventType) Valid() bool {
	switch e {
	case EventTypeClaimed, EventTypeStarted, EventTypeHeartbeat,
		EventTypeCompleted, EventTypeFailed, EventTypeRecovered:
		return true
	}
	return false
}

// --- Time Types ---

// SQLiteTime wraps time.Time to handle SQLite TEXT datetime format
type SQLiteTime struct {
	time.Time
}

// Scan implements sql.Scanner for SQLite TEXT datetime
func (st *SQLiteTime) Scan(value any) error {
	if value == nil {
		st.Time = time.Time{}
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into SQLiteTime", value)
	}

	// SQLite datetime format: "2006-01-02 15:04:05"
	t, err := time.Parse("2006-01-02 15:04:05", str)
	if err != nil {
		return err
	}

	st.Time = t
	return nil
}

// MarshalJSON implements json.Marshaler for SQLiteTime
func (st SQLiteTime) MarshalJSON() ([]byte, error) {
	return st.Time.MarshalJSON()
}

// NullSQLiteTime wraps sql.NullTime to handle SQLite TEXT datetime format
type NullSQLiteTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements sql.Scanner for nullable SQLite TEXT datetime
func (nst *NullSQLiteTime) Scan(value any) error {
	if value == nil {
		nst.Time = time.Time{}
		nst.Valid = false
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into NullSQLiteTime", value)
	}

	t, err := time.Parse("2006-01-02 15:04:05", str)
	if err != nil {
		return err
	}

	nst.Time = t
	nst.Valid = true
	return nil
}

// MarshalJSON implements json.Marshaler for NullSQLiteTime
func (nst NullSQLiteTime) MarshalJSON() ([]byte, error) {
	if !nst.Valid {
		return []byte("null"), nil
	}
	return nst.Time.MarshalJSON()
}

// Value implements driver.Valuer for NullSQLiteTime
func (nst NullSQLiteTime) Value() (driver.Value, error) {
	if !nst.Valid {
		return nil, nil
	}
	return nst.Time.Format("2006-01-02 15:04:05"), nil
}
