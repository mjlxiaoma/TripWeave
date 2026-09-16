package planner

import (
	"context"
	"log/slog"
	"sync"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/location"
)

// locateConcurrency bounds concurrent provider calls while auto-locating a
// freshly generated itinerary (keeps bursts within the daily quota's courtesy).
const locateConcurrency = 4

// locatableTypes are the activity kinds worth a provider lookup; "自由活动" or
// "市内交通" titles geocode to noise, so transport/free_time/other stay bare.
var locatableTypes = map[string]bool{
	"attraction": true, "restaurant": true, "cafe": true, "hotel": true,
}

// locateActivities attaches locations to freshly created activities in the
// background: it never blocks tool results, and a per-item failure is logged
// and skipped (the map panel falls back to live search for unlocated rows).
// locator may be nil (map provider not configured) — a no-op then.
func (e *Engine) locateActivities(acts []day.Activity, city string) {
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
		sem := make(chan struct{}, locateConcurrency)
		var wg sync.WaitGroup
		for _, a := range items {
			wg.Add(1)
			sem <- struct{}{}
			go func(act day.Activity) {
				defer wg.Done()
				defer func() { <-sem }()
				e.locateOne(ctx, act, city)
			}(a)
		}
		wg.Wait()
	}()
}

func (e *Engine) locateOne(ctx context.Context, act day.Activity, city string) {
	loc, err := e.locator.Locate(ctx, act.Title, city)
	if err != nil {
		if err != location.ErrNoProvider {
			slog.Warn("auto-locate failed", "activity", act.ID, "title", act.Title, "error", err)
		}
		return
	}
	if loc == nil {
		return
	}
	if _, err := e.days.UpdateActivity(ctx, act.ID, day.ActivityPatch{LocationID: &loc.ID}); err != nil {
		slog.Warn("auto-locate bind failed", "activity", act.ID, "location", loc.ID, "error", err)
	}
}

// cityOf extracts the trip destination as the geocoding bias ("" when unset).
func cityOf(ec *ExecContext) string {
	if ec.Trip != nil && ec.Trip.Trip != nil && ec.Trip.Trip.Destination != nil {
		return *ec.Trip.Trip.Destination
	}
	return ""
}
