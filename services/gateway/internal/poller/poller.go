package poller

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	commonv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Source represents an external data source to poll.
type Source interface {
	Name() string
	Poll(ctx context.Context) ([]RawItem, error)
}

// RawItem represents a raw item from a polled source.
type RawItem struct {
	Content  string
	SourceID string
	Metadata map[string]string
}

// Poller periodically checks external sources for new data.
type Poller struct {
	logger   *slog.Logger
	sources  []Source
	interval time.Duration
	itemChan chan *ingestionv1.InboxItem
}

// New creates a new Poller.
func New(logger *slog.Logger, interval time.Duration) *Poller {
	return &Poller{
		logger:   logger,
		sources:  make([]Source, 0),
		interval: interval,
		itemChan: make(chan *ingestionv1.InboxItem, 100),
	}
}

// AddSource registers a new polling source.
func (p *Poller) AddSource(source Source) {
	p.sources = append(p.sources, source)
}

// Items returns the channel of polled inbox items.
func (p *Poller) Items() <-chan *ingestionv1.InboxItem {
	return p.itemChan
}

// Start begins polling all registered sources.
func (p *Poller) Start(ctx context.Context) {
	p.logger.Info("starting pollers", "sources", len(p.sources), "interval", p.interval)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Initial poll
	p.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("pollers stopped")
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	for _, source := range p.sources {
		items, err := source.Poll(ctx)
		if err != nil {
			p.logger.Error("poll failed", "source", source.Name(), "error", err)
			continue
		}

		for _, raw := range items {
			item := &ingestionv1.InboxItem{
				Id:          uuid.New().String(),
				Content:     raw.Content,
				Source:      source.Name(),
				SourceId:    raw.SourceID,
				ReceivedAt:  timestamppb.New(time.Now()),
				RawMetadata: raw.Metadata,
				Priority:    commonv1.Priority_PRIORITY_NORMAL,
				ContentType: "text/plain",
			}

			select {
			case p.itemChan <- item:
			default:
				p.logger.Warn("item channel full, dropping polled item", "source", source.Name())
			}
		}

		p.logger.Info("poll complete", "source", source.Name(), "items", len(items))
	}
}
