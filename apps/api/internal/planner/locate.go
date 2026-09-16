package planner

import (
	"context"
	"log/slog"
	"time"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/location"
)

// locateInterval throttles auto-locate lookups. Amap personal-tier Web
// Service keys are capped at ~1 QPS for POI search (CUQPS errors observed
// even at concurrency 2), so we run serially with a gap. A QPS failure gets
// exactly one retry after a longer backoff.
const (
	locateInterval     = 300 * time.Millisecond
	locateRetryBackoff = 800 * time.Millisecond
)

// locatableTypes are the activity kinds worth a provider lookup; "自由活动" or
// "市内交通" titles geocode to noise, so transport/free_time/other stay bare.
var locatableTypes = map[string]bool{
	"attraction": true, "restaurant": true, "cafe": true, "hotel": true,
}

// locateActivities attaches locations to freshly created activities in the
// background: it never blocks tool results, and a per-item failure is logged
// and skipped (the map panel falls back to live search for unlocated rows).
// locator may be nil (map provider not configured) — a no-op then.
//
// Anchoring: the destination centroid seeds the anchor set, and each located
// activity becomes an anchor for the next. Generic titles ("卧龙镇午餐") match
// same-keyword POIs nationwide; the anchor chain rejects candidates that sit
// implausibly far from the itinerary instead of binding them wrong.
func (e *Engine) locateActivities(acts []day.Activity, city string, existing []location.Point) {
	if e.locator == nil {
		return
	}
	items := make([]day.Activity, 0, len(acts))
	for _, a := range acts {
		if a.LocationID == nil && locatableTypes[a.Type] {
			items = append(items, a)
		}
	}
	if len(items) == 0 {
		return
	}
	go func() {
		// Detached from the request: the SSE turn may end long before lookups
		// finish, and each lookup persists independently of the others.
		ctx := context.Background()
		anchors := existing
		if city != "" {
			if p, err := e.locator.Anchor(ctx, city); err == nil && p != nil {
				anchors = append(anchors, *p)
			}
		}
		for i, a := range items {
			if i > 0 {
				time.Sleep(locateInterval)
			}
			if p := e.locateOne(ctx, a, city, anchors); p != nil {
				anchors = append(anchors, *p)
			}
		}
	}()
}

// anchorsFromDays collects the trip's already-located activity points so
// single-activity creation is anchored to the existing itinerary too.
func anchorsFromDays(days []day.Day) []location.Point {
	var pts []location.Point
	for _, d := range days {
		for _, a := range d.Activities {
			if a.Location != nil {
				pts = append(pts, location.Point{Latitude: a.Location.Latitude, Longitude: a.Location.Longitude})
			}
		}
	}
	return pts
}

func (e *Engine) locateOne(ctx context.Context, act day.Activity, city string, anchors []location.Point) *location.Point {
	loc, err := e.locateWithRetry(ctx, act.Title, city, anchors)
	if err != nil {
		if err != location.ErrNoProvider {
			slog.Warn("auto-locate failed", "activity", act.ID, "title", act.Title, "error", err)
		}
		return nil
	}
	if loc == nil {
		return nil
	}
	if _, err := e.days.UpdateActivity(ctx, act.ID, day.ActivityPatch{LocationID: &loc.ID}); err != nil {
		slog.Warn("auto-locate bind failed", "activity", act.ID, "location", loc.ID, "error", err)
		return nil
	}
	return &location.Point{Latitude: loc.Latitude, Longitude: loc.Longitude}
}

func (e *Engine) locateWithRetry(ctx context.Context, title, city string, anchors []location.Point) (*location.Location, error) {
	loc, err := e.locator.Locate(ctx, title, city, anchors)
	if err != nil && location.IsQPSLimited(err) {
		time.Sleep(locateRetryBackoff)
		loc, err = e.locator.Locate(ctx, title, city, anchors)
	}
	return loc, err
}

// cityOf extracts the trip destination as the geocoding bias ("" when unset).
func cityOf(ec *ExecContext) string {
	if ec.Trip != nil && ec.Trip.Trip != nil && ec.Trip.Trip.Destination != nil {
		return *ec.Trip.Trip.Destination
	}
	return ""
}
