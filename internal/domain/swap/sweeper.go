package swap

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Sweeper periodically runs SweepExpiredOffers, releasing escrow on-chain for
// created swap offers past their expires_at and marking them expired (#243).
// Without it, stale offers and the escrowed funds behind them are never
// cleaned up.
type Sweeper struct {
	service  *Service
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewSweeper(service *Service, interval time.Duration) *Sweeper {
	return &Sweeper{
		service:  service,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the sweep loop in the background. The loop stops when ctx is
// cancelled or Stop is called.
func (s *Sweeper) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
}

// Stop signals the sweeper to stop after the current sweep completes.
func (s *Sweeper) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Sweeper) run(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	log.Info().Dur("interval", s.interval).Msg("swap sweep worker started")

	for {
		select {
		case <-ticker.C:
			s.sweep(ctx)
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		}
	}
}

func (s *Sweeper) sweep(ctx context.Context) {
	swept, err := s.service.SweepExpiredOffers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("swap sweep failed")
		return
	}
	if swept > 0 {
		log.Info().Int("swept", swept).Msg("swap sweep released expired offers")
	}
}
