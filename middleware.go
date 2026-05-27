// Package traefikgeoip is a Traefik plugin for Maxmind GeoIP2.
package traefikgeoip

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"

	lib "github.com/infrana/traefikgeoip/lib"
)

// Package-level singletons: the MaxMind DB is ~120 MB and must be opened once,
// not once per middleware instance (i.e. per route). Opening per-instance when
// a global middleware is applied to hundreds of routes causes an OOMKill on startup.
// This mirrors the pattern used in the upstream traefik-plugins/traefikgeoip2.
var (
	singletonCity    lib.LookupGeoIPCity
	singletonCountry lib.LookupGeoIPCountry
	singletonAsn     lib.LookupGeoIPAsn
	singletonMu      sync.Mutex
)

// ResetLookup resets cached DB readers. Used in tests to force re-initialisation.
func ResetLookup() {
	singletonMu.Lock()
	defer singletonMu.Unlock()
	singletonCity = nil
	singletonCountry = nil
	singletonAsn = nil
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *lib.Config {
	return &lib.Config{}
}

// New created a new TraefikGeoIP plugin.
//
//nolint:gocyclo
func New(_ context.Context, next http.Handler, cfg *lib.Config, name string) (http.Handler, error) {
	lookupCity, lookupCountry, lookupAsn, err := getOrCreateLookups(cfg, name)
	if err != nil {
		if cfg.FailInError {
			log.Fatalf("%s", err.Error())
		}

		stderrLogger := log.New(os.Stderr, "ERROR: ", log.LstdFlags|log.Lshortfile)
		stderrLogger.Printf("%s. Only processing IpHeader.", err.Error())
		return &lib.TraefikGeoIP{
			Next:    next,
			Name:    name,
			Options: lib.ConfigToOptions(cfg),
		}, nil
	}

	switch {
	case cfg.LightMode && lookupCity != nil && lookupAsn != nil:
		return &lib.TraefikGeoIPCityAsnLightMode{
			Next:       next,
			Name:       name,
			Options:    lib.ConfigToOptions(cfg),
			LookupAsn:  lookupAsn,
			LookupCity: lookupCity,
		}, nil

	case lookupCity != nil && lookupAsn != nil:
		return &lib.TraefikGeoIPCityAsn{
			Next:       next,
			Name:       name,
			Options:    lib.ConfigToOptions(cfg),
			LookupAsn:  lookupAsn,
			LookupCity: lookupCity,
		}, nil
	case cfg.LightMode && lookupCity != nil:
		return &lib.TraefikGeoIPCityLightMode{
			Next:       next,
			Name:       name,
			Options:    lib.ConfigToOptions(cfg),
			LookupCity: lookupCity,
		}, nil
	case lookupCity != nil:
		return &lib.TraefikGeoIPCity{
			Next:       next,
			Name:       name,
			Options:    lib.ConfigToOptions(cfg),
			LookupCity: lookupCity,
		}, nil
	case lookupCountry != nil && lookupAsn != nil:
		return &lib.TraefikGeoIPCountryAsn{
			Next:          next,
			Name:          name,
			Options:       lib.ConfigToOptions(cfg),
			LookupAsn:     lookupAsn,
			LookupCountry: lookupCountry,
		}, nil
	case lookupCountry != nil:
		return &lib.TraefikGeoIPCountry{
			Next:          next,
			Name:          name,
			Options:       lib.ConfigToOptions(cfg),
			LookupCountry: lookupCountry,
		}, nil
	case lookupAsn != nil:
		return &lib.TraefikGeoIPAsn{
			Next:      next,
			Name:      name,
			Options:   lib.ConfigToOptions(cfg),
			LookupAsn: lookupAsn,
		}, nil
	default:
		return &lib.TraefikGeoIPNotFound{
			Next:    next,
			Name:    name,
			Options: lib.ConfigToOptions(cfg),
		}, nil
	}
}

// getOrCreateLookups returns cached DB readers, opening each DB at most once.
// Concurrent calls are serialised by singletonMu so the DB is never opened twice.
func getOrCreateLookups(cfg *lib.Config, name string) (lib.LookupGeoIPCity, lib.LookupGeoIPCountry, lib.LookupGeoIPAsn, error) {
	singletonMu.Lock()
	defer singletonMu.Unlock()

	if singletonCity == nil && cfg.CityDBPath != "" {
		var err error
		singletonCity, err = lib.NewLookupCity(cfg.CityDBPath, name, cfg.Iso88591)
		if err != nil {
			return nil, nil, nil, err
		}
	} else if singletonCountry == nil && cfg.CountryDBPath != "" {
		var err error
		singletonCountry, err = lib.NewLookupCountry(cfg.CountryDBPath, name, cfg.Iso88591)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	if singletonAsn == nil && cfg.AsnDBPath != "" {
		var err error
		singletonAsn, err = lib.NewLookupAsn(cfg.AsnDBPath, name, cfg.Iso88591)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return singletonCity, singletonCountry, singletonAsn, nil
}
