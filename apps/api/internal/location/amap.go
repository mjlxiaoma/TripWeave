package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNoProvider is returned when no map service key is configured. Callers
// degrade gracefully (503 / skip locating), mirroring the AI provider pattern.
var ErrNoProvider = errors.New("map provider not configured")

// ErrProvider wraps any upstream Amap failure (network, HTTP status, or an
// error payload). Handlers map it to 502.
var ErrProvider = errors.New("map provider request failed")

// IsQPSLimited reports whether err is Amap's per-second rate limit
// (CUQPS_HAS_EXCEEDED_THE_LIMIT). Callers may retry after a short backoff —
// unlike the daily quota, this one recovers within a second.
func IsQPSLimited(err error) bool {
	return errors.Is(err, ErrProvider) && strings.Contains(err.Error(), "CUQPS")
}

const defaultAmapBaseURL = "https://restapi.amap.com"

// AmapClient talks to the Amap Web Service API (restapi.amap.com). All
// coordinates are GCJ-02, matching the Amap JS API used by the web frontend,
// so nothing needs conversion on either side.
type AmapClient struct {
	key     string
	baseURL string
	http    *http.Client
}

// NewAmapClient creates a client. An empty key keeps the API runnable without
// map access: every call then returns ErrNoProvider.
func NewAmapClient(key string) *AmapClient {
	return &AmapClient{
		key:     key,
		baseURL: defaultAmapBaseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// POI is one normalized place result (POI search or geocode fallback).
// ProviderPlaceID is empty for geocode results; the service layer synthesizes
// one so locations can still be deduplicated.
type POI struct {
	ProviderPlaceID string
	Name            string
	Longitude       float64
	Latitude        float64
	Address         string
	City            string
}

// Direction is one routed leg between two points.
type Direction struct {
	DistanceM float64
	DurationS float64
	Polyline  string // "lng,lat;lng,lat;..." GCJ-02, JS-API compatible
}

// flexString decodes JSON strings that Amap may return as an empty array or
// null on sparse results ("address": []), which would otherwise break decoding.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || b[0] == '[' || string(b) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*f = flexString(s)
	return nil
}

// SearchPOI runs the v5 keyword search. city biases results toward the trip
// destination but is not a hard filter (itineraries may span cities).
func (c *AmapClient) SearchPOI(ctx context.Context, keywords, city string) ([]POI, error) {
	if c == nil || c.key == "" {
		return nil, ErrNoProvider
	}
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("keywords", keywords)
	q.Set("page_size", "10")
	if city != "" {
		q.Set("city", city)
	}
	var resp struct {
		Status flexString `json:"status"`
		Info   flexString `json:"info"`
		Pois   []struct {
			ID       flexString `json:"id"`
			Name     flexString `json:"name"`
			Location flexString `json:"location"`
			Address  flexString `json:"address"`
			CityName flexString `json:"cityname"`
		} `json:"pois"`
	}
	if err := c.get(ctx, "/v5/place/text", q, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "1" {
		return nil, fmt.Errorf("%w: place/text: %s", ErrProvider, resp.Info)
	}
	out := make([]POI, 0, len(resp.Pois))
	for _, p := range resp.Pois {
		lng, lat, ok := parseLocation(string(p.Location))
		if !ok || p.ID == "" || p.Name == "" {
			continue
		}
		out = append(out, POI{
			ProviderPlaceID: string(p.ID),
			Name:            string(p.Name),
			Longitude:       lng,
			Latitude:        lat,
			Address:         string(p.Address),
			City:            string(p.CityName),
		})
	}
	return out, nil
}

// Geocode resolves an address/name string to coordinates. Results have no
// provider id and are the fallback when POI search finds nothing.
func (c *AmapClient) Geocode(ctx context.Context, address, city string) ([]POI, error) {
	if c == nil || c.key == "" {
		return nil, ErrNoProvider
	}
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("address", address)
	if city != "" {
		q.Set("city", city)
	}
	var resp struct {
		Status   flexString `json:"status"`
		Info     flexString `json:"info"`
		Geocodes []struct {
			Formatted flexString `json:"formatted_address"`
			Location  flexString `json:"location"`
			City      flexString `json:"city"`
			Province  flexString `json:"province"`
		} `json:"geocodes"`
	}
	if err := c.get(ctx, "/v3/geocode/geo", q, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "1" {
		return nil, fmt.Errorf("%w: geocode: %s", ErrProvider, resp.Info)
	}
	out := make([]POI, 0, len(resp.Geocodes))
	for _, g := range resp.Geocodes {
		lng, lat, ok := parseLocation(string(g.Location))
		if !ok || g.Formatted == "" {
			continue
		}
		resultCity := string(g.City)
		if resultCity == "" {
			resultCity = string(g.Province) // 直辖市的 city 字段是空数组
		}
		out = append(out, POI{
			Name:      string(g.Formatted),
			Longitude: lng,
			Latitude:  lat,
			Address:   string(g.Formatted),
			City:      resultCity,
		})
	}
	return out, nil
}

// WeatherCast is one day of the weather forecast (extensions=all).
type WeatherCast struct {
	Date         string `json:"date"`
	Week         string `json:"week"`
	DayWeather   string `json:"dayweather"`
	NightWeather string `json:"nightweather"`
	DayTemp      string `json:"daytemp"`
	NightTemp    string `json:"nighttemp"`
}

// Weather returns the multi-day forecast for a city (adcode or city name).
func (c *AmapClient) Weather(ctx context.Context, city string) ([]WeatherCast, error) {
	if c == nil || c.key == "" {
		return nil, ErrNoProvider
	}
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("city", city)
	q.Set("extensions", "all")
	var resp struct {
		Status    flexString `json:"status"`
		Info      flexString `json:"info"`
		Forecasts []struct {
			City  flexString    `json:"city"`
			Casts []WeatherCast `json:"casts"`
		} `json:"forecasts"`
	}
	if err := c.get(ctx, "/v3/weather/weatherInfo", q, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "1" {
		return nil, fmt.Errorf("%w: weather: %s", ErrProvider, resp.Info)
	}
	if len(resp.Forecasts) == 0 {
		return nil, nil
	}
	return resp.Forecasts[0].Casts, nil
}

// Direction routes origin → destination ("lng,lat" each). mode is "driving" or
// "walking"; both endpoints share the v5 response shape.
func (c *AmapClient) Direction(ctx context.Context, mode, origin, destination string) (*Direction, error) {
	if c == nil || c.key == "" {
		return nil, ErrNoProvider
	}
	path := "/v5/direction/driving"
	if mode == "walking" {
		path = "/v5/direction/walking"
	}
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("origin", origin)
	q.Set("destination", destination)
	q.Set("show_fields", "polyline,cost")
	var resp struct {
		Status flexString `json:"status"`
		Info   flexString `json:"info"`
		Route  struct {
			Paths []struct {
				Distance flexString `json:"distance"`
				Cost     struct {
					Duration flexString `json:"duration"`
				} `json:"cost"`
				Steps []struct {
					Polyline flexString `json:"polyline"`
				} `json:"steps"`
			} `json:"paths"`
		} `json:"route"`
	}
	if err := c.get(ctx, path, q, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "1" {
		return nil, fmt.Errorf("%w: direction/%s: %s", ErrProvider, mode, resp.Info)
	}
	if len(resp.Route.Paths) == 0 {
		return nil, fmt.Errorf("%w: direction/%s: no route", ErrProvider, mode)
	}
	p := resp.Route.Paths[0]
	polylines := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		if s.Polyline != "" {
			polylines = append(polylines, string(s.Polyline))
		}
	}
	dist, _ := strconv.ParseFloat(string(p.Distance), 64)
	dur, _ := strconv.ParseFloat(string(p.Cost.Duration), 64)
	return &Direction{
		DistanceM: dist,
		DurationS: dur,
		Polyline:  strings.Join(polylines, ";"),
	}, nil
}

// get performs one GET against the Web Service API and decodes the body.
func (c *AmapClient) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: http %d", ErrProvider, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProvider, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: bad json", ErrProvider)
	}
	return nil
}

// parseLocation splits Amap's "lng,lat" format (note: longitude first).
func parseLocation(s string) (lng, lat float64, ok bool) {
	lngStr, latStr, found := strings.Cut(s, ",")
	if !found {
		return 0, 0, false
	}
	lng, err1 := strconv.ParseFloat(strings.TrimSpace(lngStr), 64)
	lat, err2 := strconv.ParseFloat(strings.TrimSpace(latStr), 64)
	return lng, lat, err1 == nil && err2 == nil
}
