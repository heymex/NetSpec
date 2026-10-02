package enrichment

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/enrichment/taillight"
	"github.com/rs/zerolog"
)

// TaillightEnricher fetches syslog and daily summary data from Taillight.
type TaillightEnricher struct {
	client     *taillight.Client
	syslogCfg  *SyslogWindowConfig
	summaryCfg *DailySummaryConfig
	logger     zerolog.Logger
}

// NewTaillightEnricher creates a new TaillightEnricher from the config.
func NewTaillightEnricher(cfg TaillightConfig, logger zerolog.Logger) *TaillightEnricher {
	if !cfg.Enabled {
		return nil
	}

	tcfg := taillight.ClientConfig{
		Backend:       taillight.Backend(cfg.Backend),
		Enabled:       cfg.Enabled,
		BaseURL:       cfg.BaseURL,
		APIKey:        cfg.ResolvedAPIKey(),
		ElasticURL:    cfg.ElasticURL,
		ElasticAPIKey: cfg.ResolvedElasticAPIKey(),
		SyslogIndex:   cfg.SyslogIndex,
		SummaryIndex:  cfg.SummaryIndex,
	}

	return &TaillightEnricher{
		client:     taillight.NewClient(tcfg, logger),
		syslogCfg:  cfg.SyslogWindow,
		summaryCfg: cfg.DailySummary,
		logger:     logger.With().Str("enricher", "taillight-syslog").Logger(),
	}
}

// Enrich runs all configured Taillight queries in parallel.
func (e *TaillightEnricher) Enrich(ctx context.Context, hostname string) (*TaillightContext, error) {
	if e == nil || e.client == nil {
		return nil, nil
	}

	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return nil, nil
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		result     = &TaillightContext{}
		hasSyslog  = false
		hasSummary = false
	)

	// Query 1: syslog window.
	if e.syslogCfg != nil && e.syslogCfg.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			window := time.Duration(e.syslogCfg.LookbackMin) * time.Minute
			severityFilter := e.syslogCfg.SeverityFilter
			if len(severityFilter) == 0 {
				severityFilter = []string{"err", "crit", "alert", "emerg", "warning"}
			}
			maxEntries := e.syslogCfg.MaxEntries
			if maxEntries <= 0 {
				maxEntries = 50
			}

			entries, total, err := e.client.QuerySyslog(ctx, hostname, window, severityFilter, maxEntries)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				e.logger.Debug().Err(err).Str("device", hostname).Msg("taillight syslog query failed")
				return
			}
			if len(entries) > 0 {
				converted := make([]SyslogEntry, len(entries))
				for i, entry := range entries {
					converted[i] = SyslogEntry{
						Timestamp: entry.Timestamp,
						Hostname:  entry.Hostname,
						Facility:  entry.Facility,
						Severity:  entry.Severity,
						Program:   entry.Program,
						Message:   entry.Message,
					}
				}
				result.SyslogWindow = &SyslogWindow{
					Device:    hostname,
					Lookback:  window.String(),
					Entries:   converted,
					TotalHits: total,
				}
				hasSyslog = true
			}
		}()
	}

	// Query 2: daily summary.
	if e.summaryCfg != nil && e.summaryCfg.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			eventKind := e.summaryCfg.EventKind
			if eventKind == "" {
				eventKind = "netlog"
			}

			ds, err := e.client.QueryDailySummary(ctx, hostname, time.Now(), eventKind)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				e.logger.Debug().Err(err).Str("device", hostname).Msg("taillight daily summary failed")
				return
			}
			if ds != nil && (ds.TotalEvents > 0 || ds.SummaryText != "") {
				topMessages := make([]MessageCount, len(ds.TopMessages))
				for i, m := range ds.TopMessages {
					topMessages[i] = MessageCount{Message: m.Message, Count: m.Count}
				}
				result.DailySummary = &DailySummary{
					ReportDate:   ds.ReportDate,
					TotalEvents:  ds.TotalEvents,
					ErrorCount:   ds.ErrorCount,
					WarningCount: ds.WarningCount,
					TopMessages:  topMessages,
					HasAnomalies: ds.HasAnomalies,
					SummaryText:  ds.SummaryText,
				}
				hasSummary = true
			}
		}()
	}

	wg.Wait()

	if !hasSyslog && !hasSummary {
		return nil, nil
	}

	return result, nil
}
