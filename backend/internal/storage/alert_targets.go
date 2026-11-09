package storage

import "time"

type RelayUserSnapshot struct {
	ID                  int64
	Username            string
	Email               string
	Role                string
	Balance             float64
	Concurrency         int
	RPMLimit            int
	Status              string
	LastUsedAt          *time.Time
	LastUsedAtSupported bool
	CreatedAt           time.Time
	ObservedAt          time.Time
}
