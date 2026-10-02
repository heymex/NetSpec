package enrichment

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Pipeline orchestrates running multiple enrichers in parallel and merging results.
type Pipeline struct {
	netbox    *NetboxEnricher
	taillight *TaillightEnricher
	elastic   *ElasticEnricher
	timeout   time.Duration
	logger    zerolog.Logger
}

// New creates a new Pipeline from the resolved config and logger.
// Returns nil if enrichment is disabled or no sources are enabled.
func New(cfg Config, logger zerolog.Logger) *Pipeline {
	if !cfg.Enabled {
		return nil
	}

	p := &Pipeline{
		timeout: cfg.Timeout,
		logger:  logger.With().Str("component", "enrichment").Logger(),
	}

	if cfg.Netbox != nil && cfg.Netbox.Enabled {
		p.netbox = NewNetboxEnricher(*cfg.Netbox, logger)
	}
	if cfg.Taillight != nil && cfg.Taillight.Enabled {
		p.taillight = NewTaillightEnricher(*cfg.Taillight, logger)
	}
	if cfg.Elastic != nil && cfg.Elastic.Enabled {
		p.elastic = NewElasticEnricher(*cfg.Elastic, logger)
	}

	if p.netbox == nil && p.taillight == nil && p.elastic == nil {
		return nil
	}

	return p
}

// EnrichContext holds the data passed to the pipeline (minimal, no cycle risk).
type EnrichContext struct {
	DeviceName string
	AlertID    string
}

// Enrich executes all configured enrichers in parallel and returns the merged result.
// If enrichment is disabled or no sources are configured, returns nil (the alert fires un-enriched).
func (p *Pipeline) Enrich(ctx context.Context, ec *EnrichContext) *EnrichedContext {
	if p == nil || ec == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		result  = &EnrichedContext{}
		errList []error
	)

	if p.netbox != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			data, err := p.netbox.Enrich(ctx, ec.DeviceName)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				p.logger.Debug().Err(err).Str("enricher", "netbox").Str("alert_id", ec.AlertID).Msg("enricher failed")
				errList = append(errList, err)
				return
			}
			if data != nil {
				result.Sources = append(result.Sources, "netbox")
				result.Netbox = data
			}
		}()
	}

	if p.taillight != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			data, err := p.taillight.Enrich(ctx, ec.DeviceName)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				p.logger.Debug().Err(err).Str("enricher", "taillight").Str("alert_id", ec.AlertID).Msg("enricher failed")
				errList = append(errList, err)
				return
			}
			if data != nil {
				result.Sources = append(result.Sources, "taillight")
				result.Taillight = data
			}
		}()
	}

	if p.elastic != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			data, err := p.elastic.Enrich(ctx, ec.DeviceName)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				p.logger.Debug().Err(err).Str("enricher", "elastic").Str("alert_id", ec.AlertID).Msg("enricher failed")
				errList = append(errList, err)
				return
			}
			if data != nil {
				result.Sources = append(result.Sources, "elastic")
				result.Elastic = data
			}
		}()
	}

	wg.Wait()

	if len(errList) > 0 {
		p.logger.Warn().Int("errors", len(errList)).Str("alert_id", ec.AlertID).Msg("enrichment completed with errors")
	}

	return result
}
