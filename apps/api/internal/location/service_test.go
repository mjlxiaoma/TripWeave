package location

import (
	"context"
	"errors"
	"testing"
)

// --- fakes ---

type fakeProvider struct {
	pois      []POI
	geocodes  []POI
	poiErr    error
	direction *Direction
	dirErr    error
}

func (f *fakeProvider) SearchPOI(context.Context, string, string) ([]POI, error) {
	return f.pois, f.poiErr
}

func (f *fakeProvider) Geocode(context.Context, string, string) ([]POI, error) {
	return f.geocodes, nil
}

func (f *fakeProvider) Direction(context.Context, string, string, string) (*Direction, error) {
	return f.direction, f.dirErr
}

type fakeStore struct {
	upserts []string // provider place ids, in call order
	loc     *Location
	err     error
}

func (f *fakeStore) Upsert(_ context.Context, _ string, placeID string, _ POI) (*Location, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.upserts = append(f.upserts, placeID)
	if f.loc != nil {
		return f.loc, nil
	}
	return &Location{ID: "loc-" + placeID, Name: "X"}, nil
}

// --- Search ---

func TestSearchUsesPOIBeforeGeocode(t *testing.T) {
	svc := NewService(&fakeProvider{
		pois:     []POI{{ProviderPlaceID: "B001", Name: "故宫", Longitude: 116, Latitude: 39}},
		geocodes: []POI{{Name: "不应被用到"}},
	}, &fakeStore{})

	locs, err := svc.Search(context.Background(), "故宫", "北京")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(locs) != 1 || locs[0].ID != "loc-B001" {
		t.Errorf("unexpected result: %+v", locs)
	}
}

func TestSearchFallsBackToGeocode(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(&fakeProvider{
		pois:     nil,
		geocodes: []POI{{Name: "景山前街4号", Longitude: 116.4, Latitude: 39.9}},
	}, store)

	locs, err := svc.Search(context.Background(), "故宫", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("want 1 location, got %d", len(locs))
	}
	// 地理编码结果没有 provider id，服务层应合成稳定的 geo: 前缀 id
	if len(store.upserts) != 1 || len(store.upserts[0]) <= 4 || store.upserts[0][:4] != "geo:" {
		t.Errorf("want synthesized geo: id, got %v", store.upserts)
	}
}

func TestSearchSynthesizedIDIsStable(t *testing.T) {
	p := POI{Name: "X", Address: "某街1号", Longitude: 116.1, Latitude: 39.1}
	if placeIDOf(p) != placeIDOf(p) {
		t.Error("same input must produce the same id")
	}
	other := p
	other.Latitude = 39.2
	if placeIDOf(p) == placeIDOf(other) {
		t.Error("different coordinates must produce different ids")
	}
}

func TestSearchCapsResults(t *testing.T) {
	pois := make([]POI, 10)
	for i := range pois {
		pois[i] = POI{ProviderPlaceID: "P" + string(rune('0'+i)), Name: "X", Longitude: 116, Latitude: 39}
	}
	svc := NewService(&fakeProvider{pois: pois}, &fakeStore{})
	locs, err := svc.Search(context.Background(), "x", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(locs) != maxSearchResults {
		t.Errorf("want %d capped results, got %d", maxSearchResults, len(locs))
	}
}

func TestSearchPropagatesProviderError(t *testing.T) {
	svc := NewService(&fakeProvider{poiErr: ErrProvider}, &fakeStore{})
	if _, err := svc.Search(context.Background(), "x", ""); !errors.Is(err, ErrProvider) {
		t.Errorf("want ErrProvider, got %v", err)
	}
}

func TestSearchEmptyIsNotAnError(t *testing.T) {
	svc := NewService(&fakeProvider{}, &fakeStore{})
	locs, err := svc.Search(context.Background(), "不存在的地方xyz", "")
	if err != nil || len(locs) != 0 {
		t.Errorf("empty result should be (nil, []), got %v %v", locs, err)
	}
}

func TestLocateMissReturnsNilNotError(t *testing.T) {
	svc := NewService(&fakeProvider{}, &fakeStore{})
	loc, err := svc.Locate(context.Background(), "x", "")
	if err != nil || loc != nil {
		t.Errorf("miss should be (nil, nil), got %v %v", loc, err)
	}
}

// --- RouteForDay ---

func TestRouteForDayAggregatesLegs(t *testing.T) {
	svc := NewService(&fakeProvider{
		direction: &Direction{DistanceM: 1000, DurationS: 300, Polyline: "1,1;2,2"},
	}, &fakeStore{})
	pts := []ActivityPoint{
		{ID: "a", Longitude: 116.0, Latitude: 39.0},
		{ID: "b", Longitude: 116.1, Latitude: 39.1},
		{ID: "c", Longitude: 116.2, Latitude: 39.2},
	}
	route, err := svc.RouteForDay(context.Background(), "day-1", pts, "walking")
	if err != nil {
		t.Fatalf("RouteForDay: %v", err)
	}
	if route.DayID != "day-1" || route.Mode != "walking" {
		t.Errorf("bad identity: %+v", route)
	}
	if len(route.Legs) != 2 {
		t.Fatalf("want 2 legs, got %d", len(route.Legs))
	}
	if route.Legs[0].FromActivityID != "a" || route.Legs[0].ToActivityID != "b" {
		t.Errorf("bad leg ids: %+v", route.Legs[0])
	}
	if route.TotalDistanceM != 2000 || route.TotalDurationS != 600 {
		t.Errorf("bad totals: %+v", route)
	}
	if len(route.ActivityIDs) != 3 {
		t.Errorf("want 3 activity ids, got %v", route.ActivityIDs)
	}
}

func TestRouteForDaySkipsFailedLeg(t *testing.T) {
	svc := NewService(&fakeProvider{dirErr: ErrProvider}, &fakeStore{})
	pts := []ActivityPoint{{ID: "a"}, {ID: "b"}}
	route, err := svc.RouteForDay(context.Background(), "d", pts, "driving")
	if err != nil {
		t.Fatalf("leg failure must not fail the day: %v", err)
	}
	if len(route.Legs) != 0 {
		t.Errorf("failed leg should be skipped, got %+v", route.Legs)
	}
	if route.TotalDistanceM != 0 || route.TotalDurationS != 0 {
		t.Errorf("totals should be zero: %+v", route)
	}
}

func TestRouteForDayZeroOrOnePoint(t *testing.T) {
	svc := NewService(&fakeProvider{direction: &Direction{}}, &fakeStore{})
	route, err := svc.RouteForDay(context.Background(), "d", []ActivityPoint{{ID: "a"}}, "driving")
	if err != nil {
		t.Fatalf("RouteForDay: %v", err)
	}
	if len(route.Legs) != 0 || len(route.ActivityIDs) != 1 {
		t.Errorf("single point should have no legs: %+v", route)
	}
}

func TestRouteForDayCapsPoints(t *testing.T) {
	pts := make([]ActivityPoint, 30)
	for i := range pts {
		pts[i] = ActivityPoint{ID: "p"}
	}
	svc := NewService(&fakeProvider{direction: &Direction{}}, &fakeStore{})
	route, err := svc.RouteForDay(context.Background(), "d", pts, "driving")
	if err != nil {
		t.Fatalf("RouteForDay: %v", err)
	}
	if len(route.ActivityIDs) != maxRoutedPoints {
		t.Errorf("want %d capped points, got %d", maxRoutedPoints, len(route.ActivityIDs))
	}
}
