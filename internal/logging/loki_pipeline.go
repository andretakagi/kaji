package logging

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/andretakagi/kaji/internal/config"
)

type SinkResolver func() map[string]string

type LokiPipeline struct {
	store        *config.ConfigStore
	resolveSinks SinkResolver
	positions    *PositionStore

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	pusher  *LokiPusher
	running bool
	tailers map[string]context.CancelFunc
}

func NewLokiPipeline(store *config.ConfigStore, positionsPath string, resolveSinks SinkResolver) *LokiPipeline {
	return &LokiPipeline{
		store:        store,
		resolveSinks: resolveSinks,
		positions:    NewPositionStore(positionsPath),
		tailers:      make(map[string]context.CancelFunc),
	}
}

func (p *LokiPipeline) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return
	}

	cfg := p.store.Get()
	if !cfg.Loki.Enabled || cfg.Loki.Endpoint == "" {
		return
	}

	if err := p.positions.Load(); err != nil {
		log.Printf("loki pipeline: %v -- all tailers will start from the beginning of their files", err)
	}

	sinkPaths := p.resolveSinks()
	activePaths := make(map[string]bool)
	for _, sinkName := range cfg.Loki.Sinks {
		if path, ok := sinkPaths[sinkName]; ok {
			activePaths[path] = true
		}
	}
	p.positions.Cleanup(activePaths)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	lines := make(chan TaggedLine, 1000)
	batches := make(chan LokiBatch, 10)

	labels := cfg.Loki.Labels
	if labels == nil {
		labels = map[string]string{"job": "kaji"}
	}

	batcher := NewLokiBatcher(
		lines,
		batches,
		cfg.Loki.BatchSize,
		time.Duration(cfg.Loki.FlushIntervalSeconds)*time.Second,
		labels,
	)

	p.pusher = NewLokiPusher(
		cfg.Loki.Endpoint,
		cfg.Loki.BearerToken,
		cfg.Loki.TenantID,
		batches,
		p.positions,
	)

	p.wg.Add(2)
	go func() {
		defer p.wg.Done()
		batcher.Run(ctx)
	}()
	go func() {
		defer p.wg.Done()
		p.pusher.Run(ctx)
	}()

	for _, sinkName := range cfg.Loki.Sinks {
		filePath, ok := sinkPaths[sinkName]
		if !ok {
			log.Printf("loki pipeline: sink %q has no file path, skipping", sinkName)
			continue
		}
		p.startTailer(ctx, sinkName, filePath, lines)
	}

	p.running = true
	log.Printf("loki pipeline: started with %d sink(s)", len(p.tailers))
}

func (p *LokiPipeline) startTailer(ctx context.Context, sink, path string, lines chan<- TaggedLine) {
	tailerCtx, tailerCancel := context.WithCancel(ctx)
	p.tailers[sink] = tailerCancel

	tailer := NewLokiTailer(sink, path, p.positions, lines)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		tailer.Run(tailerCtx)
	}()
}

func (p *LokiPipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()

	if err := p.positions.Save(); err != nil {
		log.Printf("loki pipeline: save positions on stop: %v", err)
	}

	p.tailers = make(map[string]context.CancelFunc)
	p.running = false
	p.pusher = nil
	log.Println("loki pipeline: stopped")
}

func (p *LokiPipeline) Restart() {
	p.Stop()
	p.Start()
}

func (p *LokiPipeline) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *LokiPipeline) GetStatus() (bool, map[string]SinkStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running || p.pusher == nil {
		return false, nil
	}
	sinks := p.pusher.GetStatus()
	for name, s := range sinks {
		_, active := p.tailers[name]
		s.Tailing = active
		sinks[name] = s
	}
	return true, sinks
}

func (p *LokiPipeline) GetPusher() *LokiPusher {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pusher
}
