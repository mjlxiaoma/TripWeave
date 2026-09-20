package inspire

import (
	"context"
	"errors"
	"testing"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
)

// --- fakes ---

type fakeProvider struct {
	text  string
	err   error
	calls int
}

func (f *fakeProvider) ChatStream(context.Context, ai.ChatRequest, func(ai.StreamChunk)) (*ai.ChatResult, error) {
	return nil, errors.New("not used")
}

func (f *fakeProvider) ChatOnce(context.Context, ai.ChatRequest) (*ai.ChatResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &ai.ChatResult{Text: f.text, FinishReason: "stop"}, nil
}

// --- parseChips ---

func TestParseChipsClean(t *testing.T) {
	raw := `[{"label":"红叶摄影","sample":"11月下旬去京都拍红叶，7天"},{"label":"海岛躺平","sample":"春节去三亚躺平5天"},{"label":"古镇慢游","sample":"周末去乌镇住两晚"}]`
	chips := parseChips(raw)
	if len(chips) != 3 {
		t.Fatalf("want 3 chips, got %d", len(chips))
	}
	if chips[0].Label != "红叶摄影" {
		t.Errorf("label = %q", chips[0].Label)
	}
}

func TestParseChipsMarkdownFence(t *testing.T) {
	raw := "好的，以下是标签：\n```json\n[{\"label\":\"A\",\"sample\":\"a\"},{\"label\":\"B\",\"sample\":\"b\"},{\"label\":\"C\",\"sample\":\"c\"}]\n```\n希望对你有帮助"
	chips := parseChips(raw)
	if len(chips) != 3 {
		t.Fatalf("fence salvage failed, got %d", len(chips))
	}
}

func TestParseChipsGarbage(t *testing.T) {
	if chips := parseChips("这不是 JSON"); chips != nil {
		t.Errorf("garbage should return nil, got %v", chips)
	}
}

func TestParseChipsTooFew(t *testing.T) {
	raw := `[{"label":"A","sample":"a"}]`
	if chips := parseChips(raw); chips != nil {
		t.Errorf("fewer than 3 chips should be rejected, got %v", chips)
	}
}

func TestParseChipsTruncatesLongFields(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "字"
	}
	raw := `[{"label":"` + long + `","sample":"` + long + `"},{"label":"B","sample":"b"},{"label":"C","sample":"c"}]`
	chips := parseChips(raw)
	if len(chips) != 3 {
		t.Fatalf("want 3, got %d", len(chips))
	}
	if len([]rune(chips[0].Label)) > maxLabelLen || len([]rune(chips[0].Sample)) > maxSampleLen {
		t.Errorf("fields not truncated: %d/%d", len([]rune(chips[0].Label)), len([]rune(chips[0].Sample)))
	}
}

// --- Service ---

func TestServiceNilProviderReturnsEmpty(t *testing.T) {
	svc := NewService(nil, nil)
	chips, err := svc.Chips(context.Background(), "zh-CN")
	if err != nil || len(chips) != 0 {
		t.Errorf("nil provider should return empty, got %v %v", chips, err)
	}
}

func TestServiceCachesInMemory(t *testing.T) {
	p := &fakeProvider{text: `[{"label":"A","sample":"a"},{"label":"B","sample":"b"},{"label":"C","sample":"c"}]`}
	svc := NewService(p, nil)
	c1, err := svc.Chips(context.Background(), "zh-CN")
	if err != nil {
		t.Fatalf("Chips: %v", err)
	}
	c2, err := svc.Chips(context.Background(), "zh-CN")
	if err != nil {
		t.Fatalf("Chips: %v", err)
	}
	if p.calls != 1 {
		t.Errorf("provider called %d times, want 1 (cache hit)", p.calls)
	}
	if len(c1) != 3 || len(c2) != 3 {
		t.Errorf("chips = %v / %v", c1, c2)
	}
}

func TestServiceProviderErrorPropagates(t *testing.T) {
	p := &fakeProvider{err: errors.New("boom")}
	svc := NewService(p, nil)
	if _, err := svc.Chips(context.Background(), "zh-CN"); err == nil {
		t.Error("want error")
	}
}

func TestServiceBadOutputReturnsEmptyNotError(t *testing.T) {
	p := &fakeProvider{text: "乱码输出"}
	svc := NewService(p, nil)
	chips, err := svc.Chips(context.Background(), "zh-CN")
	if err != nil {
		t.Fatalf("bad output should not error, got %v", err)
	}
	if len(chips) != 0 {
		t.Errorf("bad output should return empty, got %v", chips)
	}
}

// --- parseDestinations ---

func TestParseDestinationsClean(t *testing.T) {
	raw := `[{"name":"九寨沟","blurb":"10月彩林巅峰期"},{"name":"毕棚沟","blurb":"雪山+红叶同屏"}]`
	d := parseDestinations(raw)
	if len(d) != 2 || d[0].Name != "九寨沟" {
		t.Fatalf("got %v", d)
	}
}

func TestParseDestinationsFence(t *testing.T) {
	raw := "```json\n[{\"name\":\"A\",\"blurb\":\"a\"},{\"name\":\"B\",\"blurb\":\"b\"}]\n```"
	if d := parseDestinations(raw); len(d) != 2 {
		t.Fatalf("fence salvage failed: %v", d)
	}
}

func TestParseDestinationsTooFew(t *testing.T) {
	if d := parseDestinations(`[{"name":"A","blurb":"a"}]`); d != nil {
		t.Errorf("fewer than 2 should be rejected, got %v", d)
	}
}

// --- Destinations service ---

func TestServiceDestinationsCachesByTheme(t *testing.T) {
	p := &fakeProvider{text: `[{"name":"A","blurb":"a"},{"name":"B","blurb":"b"}]`}
	svc := NewService(p, nil)
	if _, err := svc.Destinations(context.Background(), "秋色徒步", "zh-CN"); err != nil {
		t.Fatalf("Destinations: %v", err)
	}
	if _, err := svc.Destinations(context.Background(), "秋色徒步", "zh-CN"); err != nil {
		t.Fatalf("Destinations: %v", err)
	}
	if p.calls != 1 {
		t.Errorf("provider called %d times, want 1 (theme cache hit)", p.calls)
	}
	// 不同主题应重新生成
	if _, err := svc.Destinations(context.Background(), "海岛避寒", "zh-CN"); err != nil {
		t.Fatalf("Destinations: %v", err)
	}
	if p.calls != 2 {
		t.Errorf("new theme should regenerate, calls = %d", p.calls)
	}
}
