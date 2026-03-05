package backup

import (
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
)

// Scheduler manages scheduled backup jobs using cron.
type Scheduler struct {
	cron    *cron.Cron
	entryID cron.EntryID
}

// NewScheduler creates a new Scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		cron: cron.New(),
	}
}

// Start begins running the given backup function on the specified cron schedule.
func (s *Scheduler) Start(schedule string, backupFunc func()) error {
	id, err := s.cron.AddFunc(schedule, func() {
		log.Println("Scheduled backup starting...")
		backupFunc()
		log.Println("Scheduled backup completed.")
	})
	if err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", schedule, err)
	}
	s.entryID = id
	s.cron.Start()
	log.Printf("Backup scheduler started with schedule: %s\n", schedule)
	return nil
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Backup scheduler stopped.")
}
