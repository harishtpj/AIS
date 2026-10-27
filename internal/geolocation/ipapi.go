package geolocation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Location is provider-neutral metadata passed to policy callbacks.
type Location struct {
	Country    string
	Region     string
	RegionCode string
	City       string
	Latitude   *float64
	Longitude  *float64
	Timezone   string
	Source     string
	Available  bool
}

type cacheEntry struct {
	location Location
	expires  time.Time
}

// IPAPIResolver caches successful and unsuccessful lookups
// to minimize external lookups for multiple requests of same loc.
type IPAPIResolver struct {
	client *http.Client
	mu     sync.Mutex
	cache  map[string]cacheEntry
	ttl    time.Duration
	negTTL time.Duration
}

func NewIPAPIResolver() *IPAPIResolver {
	return &IPAPIResolver{
		client: &http.Client{Timeout: 2 * time.Second},
		cache:  make(map[string]cacheEntry),
		ttl:    15 * time.Minute,
		negTTL: 1 * time.Minute,
	}
}

func (r *IPAPIResolver) Resolve(ctx context.Context, ip net.IP) (Location, error) {
	if ip == nil || isPrivateOrSpecial(ip) {
		return Location{Source: "unavailable"}, nil
	}
	key := ip.String()
	now := time.Now()
	r.mu.Lock()
	if cached, ok := r.cache[key]; ok {
		if now.Before(cached.expires) {
			r.mu.Unlock()
			return cached.location, nil
		}
		delete(r.cache, key)
	}
	r.mu.Unlock()

	endpoint := "https://ipapi.co/" + url.PathEscape(key) + "/json/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Location{Source: "ipapi.co"}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return Location{Source: "ipapi.co"}, fmt.Errorf("ipapi.co lookup: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Location{Source: "ipapi.co"}, fmt.Errorf("ipapi.co returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		CountryCode string   `json:"country_code"`
		Region      string   `json:"region"`
		RegionCode  string   `json:"region_code"`
		City        string   `json:"city"`
		Latitude    *float64 `json:"latitude"`
		Longitude   *float64 `json:"longitude"`
		Timezone    string   `json:"timezone"`
		Error       bool     `json:"error"`
		Reason      string   `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return Location{Source: "ipapi.co"}, fmt.Errorf("decode ipapi.co response: %w", err)
	}
	loc := Location{
		Country: strings.ToUpper(payload.CountryCode), Region: payload.Region,
		RegionCode: payload.RegionCode, City: payload.City,
		Latitude: payload.Latitude, Longitude: payload.Longitude,
		Timezone: payload.Timezone, Source: "ipapi.co",
		Available: !payload.Error && payload.CountryCode != "",
	}
	if payload.Error {
		loc = Location{Source: "ipapi.co"}
	}

	ttl := r.ttl
	if !loc.Available {
		ttl = r.negTTL
	}
	r.mu.Lock()
	r.cache[key] = cacheEntry{location: loc, expires: time.Now().Add(ttl)}
	r.mu.Unlock()
	if payload.Error {
		return loc, fmt.Errorf("ipapi.co lookup failed: %s", payload.Reason)
	}
	return loc, nil
}

func isPrivateOrSpecial(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
