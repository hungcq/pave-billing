package ext_services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"encore.app/billing/models"
	"encore.dev/rlog"
	"encore.dev/storage/cache"
)

var secrets struct {
	OpenExchangeRatesAppId string
}

//go:generate mockgen -package=mocks -destination=mocks/exchange_rates_mock.go . ExchangeRatesService
type ExchangeRatesService interface {
	GetRates(ctx context.Context) (*models.RatesData, error)
}

type service struct {
	authHeaders map[string]string
	cache       *cache.StructKeyspace[string, models.RatesData]
	client      *http.Client
	rates       map[string]float64
	updatedAt   time.Time
	cfg         *models.AppConfig
}

type exrResponse struct {
	Base       string             `json:"base"`
	Disclaimer string             `json:"disclaimer"`
	License    string             `json:"license"`
	Rates      map[string]float64 `json:"rates"`
	Timestamp  int64              `json:"timestamp"`
}

func NewConversionService(cfg *models.AppConfig, cache *cache.StructKeyspace[string, models.RatesData]) *service {
	log := rlog.With("module", "exchange_rates_service")
	log.Info("conversion service initialized", "cache_available", cache != nil)

	return &service{
		cache:  cache,
		client: &http.Client{},
		cfg:    cfg,
	}
}

func (s *service) GetRates(
	ctx context.Context,
) (*models.RatesData, error) {
	log := rlog.With("module", "exchange_rates_service")
	log.Info("getting exchange rates")

	if err := s.updateRates(ctx); err != nil {
		log.Error("failed to update exchange rates", "error", err)
		return nil, err
	}

	return &models.RatesData{Rates: s.rates, UpdatedAt: s.updatedAt}, nil
}

// updateRates updates the exchange rates once every configured TTL by fetching the latest rates from the open exchange rates API
// returns data from cache if it's not expired
func (s *service) updateRates(ctx context.Context) error {
	log := rlog.With("module", "exchange_rates_service")

	// Use configured TTL
	ttl := time.Duration(s.cfg.ExternalServices.ExchangeRates.TTL()) * time.Second

	if time.Now().Before(s.updatedAt.Add(ttl)) {
		log.Debug("exchange rates are still fresh", "rates_updated_at", s.updatedAt, "ttl", ttl)
		return nil
	}

	log.Info("exchange rates expired, updating from cache or API")

	// Use configured cache key
	cacheKey := s.cfg.ExternalServices.ExchangeRates.CacheKey()
	data, err := s.cache.Get(ctx, cacheKey)
	if err == nil {
		log.Info("retrieved exchange rates from cache", "rates_count", len(data.Rates), "cache_updated_at", data.UpdatedAt)
		s.rates = data.Rates
		s.updatedAt = data.UpdatedAt
		return nil
	}

	log.Info("cache miss, fetching exchange rates from API")
	exr, err := s.fetchExchangeRates(ctx)
	if err != nil {
		log.Error("failed to fetch exchange rates from API", "error", err)
		return err
	}

	s.rates = exr.Rates
	s.updatedAt = time.Now().Add(ttl)

	log.Info("fetched new exchange rates from API",
		"rates_count", len(s.rates),
		"base_currency", exr.Base,
		"api_timestamp", exr.Timestamp,
		"new_ttl_expiry", s.updatedAt)

	// Cache the new rates
	err = s.cache.Set(ctx, cacheKey, models.RatesData{
		Rates:     s.rates,
		UpdatedAt: s.updatedAt,
	})
	if err != nil {
		log.Warn("failed to cache exchange rates", "error", err)
		// Don't return error as the rates are still available in memory
	}

	return nil
}

func (s *service) fetchExchangeRates(ctx context.Context) (exrResponse, error) {
	// Use configured base URL
	baseURL := s.cfg.ExternalServices.ExchangeRates.BaseURL()

	log := rlog.With("module", "exchange_rates_service").With("external_service", "openexchangerates").With("endpoint", baseURL)
	log.Info("fetching exchange rates from external API")

	// Use configured timeout
	timeout := time.Duration(s.cfg.ExternalServices.ExchangeRates.Timeout()) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?app_id="+secrets.OpenExchangeRatesAppId, nil)
	if err != nil {
		log.Error("failed to create HTTP request", "error", err)
		return exrResponse{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.Error("failed to execute HTTP request", "error", err)
		return exrResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error("API returned non-OK status", "status_code", resp.StatusCode)
		return exrResponse{}, fmt.Errorf("failed to call exchange rate service, status code: %d", resp.StatusCode)
	}

	exr := exrResponse{}
	if err = json.NewDecoder(resp.Body).Decode(&exr); err != nil {
		log.Error("failed to decode API response", "error", err)
		return exrResponse{}, err
	}

	log.Info("successfully fetched exchange rates",
		"base_currency", exr.Base,
		"rates_count", len(exr.Rates),
		"api_timestamp", exr.Timestamp,
		"license", exr.License)

	return exr, nil
}
