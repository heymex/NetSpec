package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/rs/zerolog"
)

func TestHandleNotificationTestNeedsConfig(t *testing.T) {
	t.Parallel()
	log := zerolog.New(io.Discard)
	eng := alerter.NewEngine(minConfigForTestNotify(), notifier.NewNotifier(log, nil), log, "")
	srv := NewServer(eng, log, "8088")
	// deliberately no SetConfig
	req := httptest.NewRequest(http.MethodPost, "/api/notifications/test", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	srv.handleNotificationTest(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func minConfigForTestNotify() *config.Config {
	return &config.Config{
		Alerts: config.AlertsConfig{
			Channels: map[string]config.ChannelConfig{},
			AlertBehavior: config.AlertBehavior{
				DeduplicationWindow: 5 * time.Minute,
			},
		},
	}
}
