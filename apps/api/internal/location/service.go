package location

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
)

// providerAmap is the locations.provider value for all Amap-sourced rows.
const providerAmap = "amap"

// maxSearchResults caps how many provider hits we persist per search; the
// frontend only needs the first handful for markers and binding.
const maxSearchResults = 5

// maxRoutedPoints caps route aggregation: a day longer than this still shows
// all markers but only the first N points get a routed polyline.
const maxRoutedPoints = 21

// Store is the narrow persistence the service needs (satisfied by Repository;
// faked in tests).
type Store interface {
	Upsert(ctx context.Context, provider, placeID string, p POI) (*Location, error)
}

// Provider is the narrow map-API surface the service needs (satisfied by
// AmapClient; faked in tests).
type Provider interface {
	SearchPOI(ctx context.Context, keywords, city string) ([]POI, error)
	Geocode(ctx context.Context, address, city string) ([]POI, error)
	Direction(ctx context.Context, mode, origin, destination string) (*Direction, error)
}

// Service orchestrates provider lookups, persistence and route aggregation.
type Service struct {
	amap Provider
	repo Store
}

// NewService creates the location service. amap may be constructed with an
// empty key (calls then return ErrNoProvider) so the API stays up unconfigured.
func NewService(amap Provider, repo Store) *Service {
	return &Service{amap: amap, repo: repo}
}

// Search resolves a free-text query to stored locations: POI search first,
// geocode fallback, everything upserted into the shared locations table.
func (s *Service) Search(ctx context.Context, q, city string) ([]Location, error) {
	if s == nil || s.amap == nil {
		return nil, ErrNoProvider
	}
	pois, err := s.amap.SearchPOI(ctx, q, city)
	if err != nil {
		return nil, err
	}
	if len(pois) == 0 {
		if pois, err = s.amap.Geocode(ctx, q, city); err != nil {
			return nil, err
		}
	}
	if len(pois) > maxSearchResults {
		pois = pois[:maxSearchResults]
	}
	out := make([]Location, 0, len(pois))
	for _, p := range pois {
		loc, err := s.repo.Upsert(ctx, providerAmap, placeIDOf(p), p)
		if err != nil {
			return nil, err
		}
		out = append(out, *loc)
	}
	return out, nil
}

// Locate returns the best single match for an activity title, or nil when the
// provider knows nothing about it — a miss is not an error (map fallback).
func (s *Service) Locate(ctx context.Context, title, city string) (*Location, error) {
	res, err := s.Search(ctx, title, city)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, nil
	}
	return &res[0], nil
}

// placeIDOf returns the provider place id, synthesizing a stable one from the
// geocode result (which has no id) so it can still be deduplicated.
func placeIDOf(p POI) string {
	if p.ProviderPlaceID != "" {
		return p.ProviderPlaceID
	}
	key := p.Address + "|" + strconv.FormatFloat(p.Longitude, 'f', 6, 64) + "," + strconv.FormatFloat(p.Latitude, 'f', 6, 64)
	sum := sha256.Sum256([]byte(key))
	return "geo:" + hex.EncodeToString(sum[:16])
}

// ActivityPoint is one located activity, in itinerary order, for routing.
type ActivityPoint struct {
	ID        string
	Title     string
	Longitude float64
	Latitude  float64
}

// RouteLeg is one routed segment between consecutive activities.
type RouteLeg struct {
	FromActivityID string `json:"from_activity_id"`
	ToActivityID   string `json:"to_activity_id"`
	DistanceM      int    `json:"distance_m"`
	DurationS      int    `json:"duration_s"`
	Polyline       string `json:"polyline"`
}

// DayRoute is the routed itinerary of one day.
type DayRoute struct {
	DayID          string     `json:"day_id"`
	Mode           string     `json:"mode"`
	ActivityIDs    []string   `json:"activity_ids"`
	Legs           []RouteLeg `json:"legs"`
	TotalDistanceM int        `json:"total_distance_m"`
	TotalDurationS int        `json:"total_duration_s"`
}

// RouteForDay routes consecutive points with the given mode ("driving" /
// "walking"). A single failed leg is logged and skipped rather than failing
// the whole day: markers plus a partial route beat a 502.
func (s *Service) RouteForDay(ctx context.Context, dayID string, pts []ActivityPoint, mode string) (*DayRoute, error) {
	if s == nil || s.amap == nil {
		return nil, ErrNoProvider
	}
	if len(pts) > maxRoutedPoints {
		pts = pts[:maxRoutedPoints]
	}
	route := &DayRoute{
		DayID:       dayID,
		Mode:        mode,
		ActivityIDs: make([]string, 0, len(pts)),
		Legs:        []RouteLeg{},
	}
	for _, p := range pts {
		route.ActivityIDs = append(route.ActivityIDs, p.ID)
	}
	for i := 0; i+1 < len(pts); i++ {
		from, to := pts[i], pts[i+1]
		d, err := s.amap.Direction(ctx, mode, coord(from), coord(to))
		if err != nil {
			slog.Warn("route leg failed, skipping", "day", dayID, "from", from.ID, "to", to.ID, "error", err)
			continue
		}
		route.Legs = append(route.Legs, RouteLeg{
			FromActivityID: from.ID,
			ToActivityID:   to.ID,
			DistanceM:      int(d.DistanceM + 0.5),
			DurationS:      int(d.DurationS + 0.5),
			Polyline:       d.Polyline,
		})
		route.TotalDistanceM += int(d.DistanceM + 0.5)
		route.TotalDurationS += int(d.DurationS + 0.5)
	}
	return route, nil
}

// coord formats a point as Amap's "lng,lat" argument.
func coord(p ActivityPoint) string {
	return fmt.Sprintf("%.6f,%.6f", p.Longitude, p.Latitude)
}
