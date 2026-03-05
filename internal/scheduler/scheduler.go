package scheduler

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

// Job defines a named scheduled job.
type Job struct {
	Name     string
	Schedule string // cron expression
	Func     func()
}

// Scheduler manages multiple named cron jobs.
type Scheduler struct {
	cron    *cron.Cron
	entries map[string]cron.EntryID
	mu      sync.Mutex
}

// New creates a new Scheduler.
func New() *Scheduler {
	return &Scheduler{
		cron:    cron.New(),
		entries: make(map[string]cron.EntryID),
	}
}

// Add registers a named job on the given cron schedule.
func (s *Scheduler) Add(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[job.Name]; exists {
		return fmt.Errorf("job %q already registered", job.Name)
	}

	fn := job.Func
	name := job.Name
	id, err := s.cron.AddFunc(job.Schedule, func() {
		slog.Info("Scheduled job starting", "job", name)
		fn()
		slog.Info("Scheduled job completed", "job", name)
	})
	if err != nil {
		return fmt.Errorf("invalid cron schedule %q for job %q: %w", job.Schedule, job.Name, err)
	}

	s.entries[job.Name] = id
	slog.Info("Scheduled job registered", "job", job.Name, "schedule", job.Schedule)
	return nil
}

// Remove unregisters a named job.
func (s *Scheduler) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.entries[name]; ok {
		s.cron.Remove(id)
		delete(s.entries, name)
		slog.Info("Scheduled job removed", "job", name)
	}
}

// Start begins running all registered jobs.
func (s *Scheduler) Start() {
	s.cron.Start()
	slog.Info("Scheduler started", "jobs", len(s.entries))
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {
	s.cron.Stop()
	slog.Info("Scheduler stopped")
}
