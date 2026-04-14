package logging

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/andretakagi/kaji/internal/config"
)

type SinkResolver func() map[string]string

type tailerHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type LokiPipeline struct {
	store        *config.ConfigStore
	resolveSinks SinkResolver
	positions    *PositionStore

	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	tailerWg sync.WaitGroup
	pusher   *LokiPusher
	lines    chan TaggedLine
	running  bool
	tailers  map[string]tailerHandle

	activeEndpoint    string
	activeBearerToken string
	activeTenantID    string
}

func NewLokiPipeline(store *config.ConfigStore, positionsPath string, resolveSinks SinkResolver) *LokiPipeline {
	return &LokiPipeline{
		store:        store,
		resolveSinks: resolveSinks,
		positions:    NewPositionStore(positionsPath),
		tailers:      make(map[string]tailerHandle),
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
	p.ctx = ctx
	p.cancel = cancel

	lines := make(chan TaggedLine, 1000)
	p.lines = lines
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
	)
	p.activeEndpoint = cfg.Loki.Endpoint
	p.activeBearerToken = cfg.Loki.BearerToken
	p.activeTenantID = cfg.Loki.TenantID

	p.wg.Add(3)
	go func() {
		defer p.wg.Done()
		batcher.Run(ctx)
	}()
	go func() {
		defer p.wg.Done()
		p.pusher.Run(ctx)
	}()
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := p.positions.Save(); err != nil {
					log.Printf("loki pipeline: save positions: %v", err)
				}
			}
		}
	}()

	for _, sinkName := range cfg.Loki.Sinks {
		filePath, ok := sinkPaths[sinkName]
		if !ok {
			log.Printf("loki pipeline: sink %q has no file path, skipping", sinkName)
			continue
		}
		p.startTailer(ctx, sinkName, filePath, lines)
	}

	if len(p.tailers) == 0 {
		log.Println("loki pipeline: no sinks resolved, shutting down")
		close(lines)
		cancel()
		p.wg.Wait()
		return
	}

	p.running = true
	log.Printf("loki pipeline: started with %d sink(s)", len(p.tailers))
}

func (p *LokiPipeline) startTailer(ctx context.Context, sink, path string, lines chan<- TaggedLine) {
	tailerCtx, tailerCancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.tailers[sink] = tailerHandle{cancel: tailerCancel, done: done}

	tailer := NewLokiTailer(sink, path, p.positions, lines)
	p.tailerWg.Add(1)
	go func() {
		defer p.tailerWg.Done()
		defer close(done)
		tailer.Run(tailerCtx)
	}()
}

func (p *LokiPipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	for _, h := range p.tailers {
		h.cancel()
	}
	p.tailerWg.Wait()

	close(p.lines)

	drainDone := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(drainDone)
	}()
	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		log.Println("loki pipeline: drain deadline exceeded, forcing shutdown")
		p.cancel()
	}
	<-drainDone

	if err := p.positions.Save(); err != nil {
		log.Printf("loki pipeline: save positions on stop: %v", err)
	}

	p.tailers = make(map[string]tailerHandle)
	p.running = false
	p.pusher = nil
	p.lines = nil
	p.ctx = nil
	log.Println("loki pipeline: stopped")
}

func (p *LokiPipeline) Reconfigure() {
	p.mu.Lock()
	cfg := p.store.Get()

	if !p.running || !cfg.Loki.Enabled {
		p.mu.Unlock()
		p.Restart()
		return
	}

	if cfg.Loki.Endpoint != p.activeEndpoint ||
		cfg.Loki.BearerToken != p.activeBearerToken ||
		cfg.Loki.TenantID != p.activeTenantID {
		p.mu.Unlock()
		log.Println("loki pipeline: connection settings changed, restarting")
		p.Restart()
		return
	}

	sinkPaths := p.resolveSinks()
	wanted := make(map[string]string)
	for _, name := range cfg.Loki.Sinks {
		if path, ok := sinkPaths[name]; ok {
			wanted[name] = path
		}
	}

	var removed []chan struct{}
	for name, h := range p.tailers {
		if _, ok := wanted[name]; !ok {
			h.cancel()
			removed = append(removed, h.done)
			delete(p.tailers, name)
			log.Printf("loki pipeline: stopped tailer for %q", name)
		}
	}

	for _, done := range removed {
		<-done
	}

	for name, path := range wanted {
		if _, ok := p.tailers[name]; !ok {
			p.startTailer(p.ctx, name, path, p.lines)
			log.Printf("loki pipeline: started tailer for %q", name)
		}
	}

	p.mu.Unlock()
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
