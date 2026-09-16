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

func (f *fakeStore) Upsert(_ context.Context, _ string, placeID string, p POI) (*Location, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.upserts = append(f.upserts, placeID)
	if f.loc != nil {
		return f.loc, nil
	}
	var city *string
	if p.City != "" {
		c := p.City
		city = &c
	}
	return &Location{ID: "loc-" + placeID, Name: p.Name, City: city, Latitude: p.Latitude, Longitude: p.Longitude}, nil
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
	loc, err := svc.Locate(context.Background(), "x", "", nil)
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

func TestCityMatches(t *testing.T) {
	cases := []struct {
		provider, dest string
		want           bool
	}{
		{"北京市", "北京", true},
		{"北京", "北京市", true},
		{"北京市延庆区", "北京", true},
		{"三亚市", "北京", false},
		{"上海市", "北京", false},
		{"北京市", "上海", false},
	}
	for _, c := range cases {
		if got := cityMatches(c.provider, c.dest); got != c.want {
			t.Errorf("cityMatches(%q,%q)=%v want %v", c.provider, c.dest, got, c.want)
		}
	}
}

func TestPreferCityPromotesSameCity(t *testing.T) {
	cityBeijing := "北京市"
	citySanya := "三亚市"
	far := Location{Name: "三亚店", City: &citySanya}
	near := Location{Name: "北京店", City: &cityBeijing}
	out := preferCity([]Location{far, near}, "北京")
	if len(out) != 2 {
		t.Fatalf("soft preference must keep all results, got %d", len(out))
	}
	if out[0].Name != "北京店" {
		t.Errorf("same-city result should come first, got %+v", out)
	}
	// 无目的地时原样返回
	out = preferCity([]Location{far, near}, "")
	if out[0].Name != "三亚店" {
		t.Errorf("empty city keeps provider order, got %+v", out)
	}
}

func TestLocateDropsCrossCityMismatch(t *testing.T) {
	// 「长城脚下农家菜」真实场景：provider 同名词条全国都有，第一个命中三亚 —
	// 目的地北京时应过滤三亚、返回北京延庆的那家。
	svc := NewService(&fakeProvider{pois: []POI{
		{ProviderPlaceID: "SY", Name: "长城脚下农家菜", City: "三亚市", Longitude: 109.6, Latitude: 18.6},
		{ProviderPlaceID: "BJ", Name: "长城脚下农家菜(延庆)", City: "北京市", Longitude: 116.0, Latitude: 40.3},
	}}, &fakeStore{})
	loc, err := svc.Locate(context.Background(), "长城脚下农家菜", "北京", nil)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if loc == nil || loc.ID != "loc-BJ" {
		t.Errorf("want Beijing match, got %+v", loc)
	}
}

func TestLocateAnchorRejectsFarCandidates(t *testing.T) {
	// 泛化标题「卧龙镇午餐」：provider 先回北京新发地、再回阿坝本地 — 有锚点时
	// 应跳过远在北京的候选、选中阿坝的；锚点正是用来防「同关键词全国错绑」。
	svc := NewService(&fakeProvider{pois: []POI{
		{ProviderPlaceID: "BJ", Name: "川味家常菜(新发地店)", City: "北京市", Longitude: 116.3, Latitude: 39.8},
		{ProviderPlaceID: "AB", Name: "卧龙镇川味家常菜", City: "阿坝藏族羌族自治州", Longitude: 103.3, Latitude: 31.1},
	}}, &fakeStore{})
	anchor := []Point{{Latitude: 31.1, Longitude: 103.3}} // 卧龙大熊猫基地
	loc, err := svc.Locate(context.Background(), "卧龙镇午餐（川味家常菜）", "", anchor)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if loc == nil || loc.ID != "loc-AB" {
		t.Errorf("anchor should skip the Beijing mismatch, got %+v", loc)
	}
}

func TestLocateAnchorAllFarReturnsNil(t *testing.T) {
	// 全部候选都远离锚点：宁可不定位，也不绑错城市。
	svc := NewService(&fakeProvider{pois: []POI{
		{ProviderPlaceID: "BJ", Name: "X", City: "北京市", Longitude: 116.3, Latitude: 39.8},
	}}, &fakeStore{})
	anchor := []Point{{Latitude: 31.1, Longitude: 103.3}}
	loc, err := svc.Locate(context.Background(), "X", "", anchor)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if loc != nil {
		t.Errorf("all-far candidates must be rejected, got %+v", loc)
	}
}

func TestLocateNoAnchorKeepsFirst(t *testing.T) {
	// 无锚点（destination 解析不出、行程还没有任何定位点）时信任 provider 排序。
	svc := NewService(&fakeProvider{pois: []POI{
		{ProviderPlaceID: "P1", Name: "X", City: "北京市", Longitude: 116.3, Latitude: 39.8},
	}}, &fakeStore{})
	loc, err := svc.Locate(context.Background(), "X", "", nil)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if loc == nil || loc.ID != "loc-P1" {
		t.Errorf("no anchors should keep the first result, got %+v", loc)
	}
}

func TestHaversineKm(t *testing.T) {
	// 北京(116.4,39.9) → 成都(104.1,30.7) 约 1520km
	d := haversineKm(39.9, 116.4, 30.7, 104.1)
	if d < 1400 || d > 1700 {
		t.Errorf("unexpected distance %.0f km", d)
	}
	if d0 := haversineKm(31.1, 103.3, 31.1, 103.3); d0 > 0.1 {
		t.Errorf("same point should be ~0, got %.4f", d0)
	}
}

func TestLocateCrossCityOnlyFallsBackToProviderOrder(t *testing.T) {
	// 跨市州行程（destination=成都，景点在阿坝）：无同城结果时不过滤，
	// 信任 provider 相关性排序返回第一个——硬过滤会把环线行程全部误杀。
	svc := NewService(&fakeProvider{pois: []POI{
		{ProviderPlaceID: "AB", Name: "卧龙大熊猫基地", City: "阿坝藏族羌族自治州", Longitude: 103.2, Latitude: 31.0},
	}}, &fakeStore{})
	loc, err := svc.Locate(context.Background(), "卧龙大熊猫基地", "成都", nil)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if loc == nil || loc.ID != "loc-AB" {
		t.Errorf("cross-prefecture match must be kept, got %+v", loc)
	}
}
