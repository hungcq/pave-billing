package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"encore.app/billing/ext_services"
	"encore.app/billing/models"
	"github.com/stretchr/testify/assert"
)

// helper to build a minimal AppConfig for the exchange rate service
func testCfg(baseURL string, ttlSeconds, timeoutSeconds int, cacheKey string) *models.AppConfig {
	return &models.AppConfig{
		ExternalServices: models.ExternalServicesConfig{
			ExchangeRates: models.ExchangeRatesConfig{
				BaseURL:  func() string { return baseURL },
				TTL:      func() int { return ttlSeconds },
				CacheKey: func() string { return cacheKey },
				Timeout:  func() int { return timeoutSeconds },
			},
		},
	}
}

func TestExchangeRatesService(t *testing.T) {
	ctx := context.Background()

	t.Run("when_cache_has_entry_should_return_cached_without_fetch", func(t *testing.T) {

		// No server needed; ensure service reads from cache
		cfg := testCfg("http://invalid.local", 300, 1, "exrates")

		// Pre-populate cache with rates
		cached := models.RatesData{
			Rates:     map[string]float64{"USD": 1.0, "GEL": 2.5},
			UpdatedAt: time.Now(),
		}
		assert.NoError(t, exchangeRatesKV.Set(ctx, cfg.ExternalServices.ExchangeRates.CacheKey(), cached))

		svc := ext_services.NewConversionService(cfg, exchangeRatesKV)
		// Force a fresh result path by calling GetRates (will load from cache because service state is empty)
		res, err := svc.GetRates(ctx)

		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, 1.0, res.Rates["USD"])
		assert.Equal(t, 2.5, res.Rates["GEL"])
		assert.False(t, res.UpdatedAt.IsZero())
	})

	t.Run("when_cache_miss_should_fetch_from_api_and_cache", func(t *testing.T) {
		// Simulate external API
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"base":       "USD",
				"disclaimer": "",
				"license":    "",
				"rates": map[string]float64{
					"USD": 1.0,
					"GEL": 2.5,
				},
				"timestamp": time.Now().Unix(),
			})
		}))
		defer server.Close()

		cfg := testCfg(server.URL, 60, 2, "exrates")
		svc := ext_services.NewConversionService(cfg, exchangeRatesKV)

		res, err := svc.GetRates(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, 1.0, res.Rates["USD"])
		assert.Equal(t, 2.5, res.Rates["GEL"])
		assert.False(t, res.UpdatedAt.IsZero())

		// Verify cached
		cached, err := exchangeRatesKV.Get(ctx, cfg.ExternalServices.ExchangeRates.CacheKey())
		assert.NoError(t, err)
		assert.Equal(t, res.Rates, cached.Rates)
	})

	t.Run("when_api_returns_non_ok_should_error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		cfg := testCfg(server.URL, 60, 1, "exrates")
		svc := ext_services.NewConversionService(cfg, exchangeRatesKV)

		res, err := svc.GetRates(ctx)
		assert.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("when_request_times_out_should_error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"base":       "USD",
				"disclaimer": "",
				"license":    "",
				"rates":      map[string]float64{"USD": 1.0},
				"timestamp":  time.Now().Unix(),
			})
		}))
		defer server.Close()

		// Set very small timeout to trigger context timeout
		cfg := testCfg(server.URL, 60, 0, "exrates") // 0s -> immediate timeout
		svc := ext_services.NewConversionService(cfg, exchangeRatesKV)

		res, err := svc.GetRates(ctx)
		assert.Error(t, err)
		assert.Nil(t, res)
	})
}
