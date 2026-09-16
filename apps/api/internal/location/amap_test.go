package location

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient points an AmapClient at the given mock server.
func newTestClient(srv *httptest.Server) *AmapClient {
	c := NewAmapClient("test-key")
	c.baseURL = srv.URL
	return c
}

func TestSearchPOIParsesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/place/text" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" || r.URL.Query().Get("keywords") != "故宫" || r.URL.Query().Get("city") != "北京" {
			t.Errorf("bad query: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{
			"status": "1", "info": "OK",
			"pois": [
				{"id": "B000A8UIN8", "name": "故宫博物院", "location": "116.397026,39.918058", "address": "景山前街4号", "cityname": "北京市"},
				{"id": "B000A81O9N", "name": "故宫角楼", "location": "116.3907,39.9224", "address": [], "cityname": "北京市"},
				{"id": "", "name": "坏行没有id", "location": "116.0,39.0"},
				{"id": "BX", "name": "坏行坐标非法", "location": "garbage"}
			]
		}`))
	}))
	defer srv.Close()

	pois, err := newTestClient(srv).SearchPOI(context.Background(), "故宫", "北京")
	if err != nil {
		t.Fatalf("SearchPOI: %v", err)
	}
	if len(pois) != 2 {
		t.Fatalf("want 2 valid pois, got %d", len(pois))
	}
	if pois[0].ProviderPlaceID != "B000A8UIN8" || pois[0].Name != "故宫博物院" {
		t.Errorf("unexpected first poi: %+v", pois[0])
	}
	if pois[0].Longitude != 116.397026 || pois[0].Latitude != 39.918058 {
		t.Errorf("bad coords: %+v", pois[0])
	}
	// address 返回空数组时应解析为空字符串而不是报错
	if pois[1].Address != "" {
		t.Errorf("array address should decode as empty, got %q", pois[1].Address)
	}
}

func TestSearchPOIErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status": "0", "info": "DAILY_QUERY_OVER_LIMIT"}`))
	}))
	defer srv.Close()
	_, err := newTestClient(srv).SearchPOI(context.Background(), "x", "")
	if !errors.Is(err, ErrProvider) {
		t.Errorf("want ErrProvider, got %v", err)
	}
}

func TestSearchPOINoKey(t *testing.T) {
	_, err := NewAmapClient("").SearchPOI(context.Background(), "x", "")
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("want ErrNoProvider, got %v", err)
	}
}

func TestGeocodeFallsBackToProvince(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/geocode/geo" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		// 直辖市的 city 字段是空数组，应回退 province
		w.Write([]byte(`{
			"status": "1",
			"geocodes": [{"formatted_address": "北京市东城区景山前街4号", "location": "116.397026,39.918058", "city": [], "province": "北京市"}]
		}`))
	}))
	defer srv.Close()

	pois, err := newTestClient(srv).Geocode(context.Background(), "故宫", "北京")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if len(pois) != 1 {
		t.Fatalf("want 1 geocode, got %d", len(pois))
	}
	if pois[0].ProviderPlaceID != "" {
		t.Errorf("geocode results must not carry a provider id, got %q", pois[0].ProviderPlaceID)
	}
	if pois[0].City != "北京市" {
		t.Errorf("city should fall back to province, got %q", pois[0].City)
	}
}

func TestDirectionJoinsStepPolylines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/direction/walking" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("origin") != "116.0,39.0" || r.URL.Query().Get("destination") != "116.1,39.1" {
			t.Errorf("bad endpoints: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{
			"status": "1",
			"route": {"paths": [{
				"distance": "1234",
				"cost": {"duration": "600"},
				"steps": [
					{"polyline": "116.0,39.0;116.01,39.0"},
					{"polyline": "116.01,39.0;116.1,39.1"}
				]
			}]}
		}`))
	}))
	defer srv.Close()

	d, err := newTestClient(srv).Direction(context.Background(), "walking", "116.0,39.0", "116.1,39.1")
	if err != nil {
		t.Fatalf("Direction: %v", err)
	}
	if d.DistanceM != 1234 || d.DurationS != 600 {
		t.Errorf("bad distance/duration: %+v", d)
	}
	if d.Polyline != "116.0,39.0;116.01,39.0;116.01,39.0;116.1,39.1" {
		t.Errorf("steps not joined: %q", d.Polyline)
	}
}

func TestDirectionNoRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status": "1", "route": {"paths": []}}`))
	}))
	defer srv.Close()
	_, err := newTestClient(srv).Direction(context.Background(), "driving", "0,0", "1,1")
	if !errors.Is(err, ErrProvider) {
		t.Errorf("want ErrProvider, got %v", err)
	}
}

func TestDirectionDrivingPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"status": "0", "info": "stop early"}`))
	}))
	defer srv.Close()
	_, _ = newTestClient(srv).Direction(context.Background(), "driving", "0,0", "1,1")
	if !strings.HasSuffix(gotPath, "/v5/direction/driving") {
		t.Errorf("driving should hit the driving endpoint, got %s", gotPath)
	}
}

func TestParseLocation(t *testing.T) {
	lng, lat, ok := parseLocation("116.397,39.908")
	if !ok || lng != 116.397 || lat != 39.908 {
		t.Errorf("parse failed: %v %v %v", lng, lat, ok)
	}
	for _, bad := range []string{"", "abc", "1.2", "1.2,"} {
		if _, _, ok := parseLocation(bad); ok {
			t.Errorf("parseLocation(%q) should fail", bad)
		}
	}
}
