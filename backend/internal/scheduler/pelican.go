package scheduler

import (
	"context"
	"time"
)

func (s *Scheduler) runPelicans() {
	root := s.pelicanCtx
	if root == nil {
		root = context.Background()
	}
	s.track(root, "pelican", 11*time.Minute, s.relay.RunDuePelicans)
}
