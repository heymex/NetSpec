package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ElasticEnricher runs standalone Elastic queries for trend analysis and
// cross-source correlation.
type ElasticEnricher struct {
	url     string
	apiKey  string
	trend   *ElasticTrendConfig
	client  *http.Client
	logger  zerolog.Logger
}

// NewElasticEnricher creates a new ElasticEnricher from the config.
func NewElasticEnricher(cfg ElasticConfig, logger zerolog.Logger) *ElasticEnricher {
	if !cfg.Enabled {
		return nil
	}

	trend := cfg.Trend
	if trend == nil || !trend.Enabled {
		// No trend config, but the enricher can still be created for future use.
	}

	return &ElasticEnricher{
		url:    strings.TrimRight(cfg.URL, "/"),
		apiKey: cfg.ResolvedAPIKey(),
		trend:  trend,
		client: &http.Client{Timeout: 15 * time.Second},
		logger: logger.With().Str("enricher", "elastic").Logger(),
	}
}

// Enrich runs the configured Elastic queries.
func (e *ElasticEnricher) Enrich(ctx context.Context, deviceName string) (*ElasticContext, error) {
	if e == nil || e.trend == nil || !e.trend.Enabled {
		return nil, nil
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		result     = &ElasticContext{}
		hasData    = false
	)

	// Trend analysis: volume and severity over the configured window.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ctx.Err() != nil {
			return
		}

		window := e.trend.Window
		if window == 0 {
			window = 15 * time.Minute
		}
		maxHits := e.trend.MaxHits
		if maxHits <= 0 {
			maxHits = 100
		}
		indexPattern := e.trend.IndexPattern
		if indexPattern == "" {
			indexPattern = "logs-*"
		}

		ta, err := e.queryTrend(ctx, deviceName, window, indexPattern, maxHits)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			e.logger.Debug().Err(err).Str("device", deviceName).Msg("elastic trend query failed")
			return
		}
		if ta != nil && ta.TotalHits > 0 {
			result.Trend = ta
			hasData = true
		}
	}()

	wg.Wait()

	if !hasData {
		return nil, nil
	}

	return result, nil
}

// queryTrend runs a time-range query against Elastic for volume and severity aggregation.
func (e *ElasticEnricher) queryTrend(ctx context.Context, device string, window time.Duration, indexPattern string, maxHits int) (*TrendAnalysis, error) {
	now := time.Now()
	fromTime := now.Add(-window).Format(time.RFC3339Nano)
	toTime := now.Format(time.RFC3339Nano)

	// Query: device hostname + time range, with severity terms aggregation.
	body := map[string]interface{}{
		"size": 0, // we only need aggregations
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"term": map[string]interface{}{"hostname": device}},
					{"range": map[string]interface{}{
						"@timestamp": map[string]interface{}{
							"gte": fromTime,
							"lte": toTime,
						},
					}},
				},
			},
		},
		"aggs": map[string]interface{}{
			"by_severity": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "severity.keyword",
					"size":  10,
				},
			},
			"top_messages": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "message",
					"size":  10,
				},
			},
		},
	}

	resp, err := e.doElasticQuery(ctx, indexPattern, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elastic trend query %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var elasticResp struct {
		Hits struct {
			Total struct{ Value int } `json:"total"`
		} `json:"hits"`
		Aggregations elasticAggs `json:"aggregations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&elasticResp); err != nil {
		return nil, err
	}

	bySeverity := make(map[string]int)
	for _, bucket := range elasticResp.Aggregations.BySeverity.Buckets {
		bySeverity[bucket.Key] = int(bucket.DocCount)
	}

	topMessages := make([]MessageCount, 0, len(elasticResp.Aggregations.TopMessages.Buckets))
	for _, bucket := range elasticResp.Aggregations.TopMessages.Buckets {
		topMessages = append(topMessages, MessageCount{
			Message: bucket.Key,
			Count:   int(bucket.DocCount),
		})
	}

	return &TrendAnalysis{
		Window:      window.String(),
		TotalHits:   elasticResp.Hits.Total.Value,
		BySeverity:  bySeverity,
		TopMessages: topMessages,
	}, nil
}

func (e *ElasticEnricher) doElasticQuery(ctx context.Context, index string, body map[string]interface{}) (*http.Response, error) {
	index = strings.TrimSpace(index)
	if index == "" {
		index = "logs-*"
	}

	reqURL := fmt.Sprintf("%s/%s/_search", e.url, url.PathEscape(index))
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+e.apiKey)
	}

	return e.client.Do(req)
}

// -- Aggregation response types --

type elasticAggs struct {
	BySeverity struct {
		Buckets []elasticBucket `json:"buckets"`
	} `json:"by_severity"`
	TopMessages struct {
		Buckets []elasticBucket `json:"buckets"`
	} `json:"top_messages"`
}

type elasticBucket struct {
	Key      string `json:"key"`
	DocCount int    `json:"doc_count"`
}
