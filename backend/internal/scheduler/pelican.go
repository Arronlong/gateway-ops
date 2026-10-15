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
	ctx, cancel := context.WithTimeout(root, 11*time.Minute)
	defer cancel()
	if err := s.relay.RunDuePelicans(ctx); err != nil {
		s.log.Error("scheduled pelican tests failed", "error", err)
	}
}
