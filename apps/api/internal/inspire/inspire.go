// Package inspire generates the homepage's travel-inspiration chips with the
// LLM and caches them (Redis 24h, in-memory fallback) so page views never hit
// the model directly. When the provider is unconfigured or the model output is
// unusable, the endpoint returns an empty list and the frontend falls back to
// its static chips.
package inspire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
)

// Chip is one inspiration tag: a short label plus a sample natural-language
// prompt the user can click to prefill the planner input.
type Chip struct {
	Label  string `json:"label"`
	Sample string `json:"sample"`
}

const (
	chipCount   = 5
	cacheTTL    = 24 * time.Hour
	genTimeout  = 20 * time.Second
	maxLabelLen = 20
	maxSampleLen = 60
)

// systemPrompt asks for exactly chipCount chips as a JSON array. Kept stable
// for provider-side prompt caching.
const systemPrompt = `你是旅行灵感助手。生成恰好 %d 个旅行灵感标签，用 JSON 数组返回，不要任何其他文字。

每个元素：{"label": "标签名", "sample": "示例话术"}

要求：
- label：4-8 个字，概括一种旅行主题（如「红叶摄影」「海岛躺平」「古镇慢游」）
- sample：一条 40 字以内的具体旅行想法，像真实用户会输入的那样，包含时间/目的地/偏好中的至少两项（如「11月下旬去京都拍红叶，7天，想住在出片的机位附近」）
- 主题要多样：自然风光、美食、人文历史、摄影、小众、周末短途等尽量覆盖
- 结合当前季节给出应景的建议
- 避免陈词滥调和过于笼统的描述

只返回 JSON 数组，不要 markdown 代码块，不要解释。`

// Destination is one recommended place for a theme: a name plus a one-line
// highlight shown under it.
type Destination struct {
	Name  string `json:"name"`
	Blurb string `json:"blurb"`
}

// Service generates and caches inspiration chips.
type Service struct {
	provider ai.Provider
	rdb      *redis.Client // nil → 进程内缓存兜底

	mu         sync.Mutex
	local      map[string]cachedChips
	localDests map[string]cachedDests
}

type cachedChips struct {
	chips     []Chip
	expiresAt time.Time
}

type cachedDests struct {
	dests     []Destination
	expiresAt time.Time
}

// NewService creates the inspire service. provider may be nil (endpoint then
// returns an empty list); rdb may be nil (in-memory cache only).
func NewService(provider ai.Provider, rdb *redis.Client) *Service {
	return &Service{provider: provider, rdb: rdb, local: map[string]cachedChips{}, localDests: map[string]cachedDests{}}
}

// Chips returns the cached chips for a locale, generating on a miss.
func (s *Service) Chips(ctx context.Context, locale string) ([]Chip, error) {
	if s.provider == nil {
		return nil, nil
	}
	if chips, ok := s.getCached(ctx, locale); ok {
		return chips, nil
	}
	chips, err := s.generate(ctx, locale)
	if err != nil {
		return nil, err
	}
	s.setCached(ctx, locale, chips)
	return chips, nil
}

// destSystemPrompt asks for 3-5 destinations for a theme as a JSON array.
const destSystemPrompt = `你是旅行目的地推荐助手。用户点选了一个旅行主题，请给出 3-5 个该主题下的真实目的地（中国为主，可以含 1 个国外），用 JSON 数组返回，不要任何其他文字。

每个元素：{"name": "目的地名", "blurb": "一句话亮点"}

要求：
- name：城市/景区名（如「九寨沟」「毕棚沟」「京都」），要真实存在
- blurb：10 字以内的一句话亮点，说明这个季节/主题下它最值得去的点（如「10月彩林巅峰期」「雪山+红叶同屏」）
- 结合当前季节推荐应景的（主题本身就带季节性的按主题，否则按当下季节）
- 目的地之间要有区分度（不同地区/风格）

只返回 JSON 数组，不要 markdown 代码块，不要解释。`

// Destinations returns cached destinations for a theme, generating on a miss.
// theme 由前端传入（用户点击的标签名）。
func (s *Service) Destinations(ctx context.Context, theme, locale string) ([]Destination, error) {
	if s.provider == nil {
		return nil, nil
	}
	key := locale + "|" + theme
	if dests, ok := s.getCachedDest(ctx, key); ok {
		return dests, nil
	}
	dests, err := s.generateDests(ctx, theme, locale)
	if err != nil {
		return nil, err
	}
	s.setCachedDest(ctx, key, dests)
	return dests, nil
}

func (s *Service) generateDests(ctx context.Context, theme, locale string) ([]Destination, error) {
	ctx, cancel := context.WithTimeout(ctx, genTimeout)
	defer cancel()

	lang := "中文"
	if strings.HasPrefix(locale, "en") {
		lang = "English"
	}
	res, err := s.provider.ChatOnce(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: destSystemPrompt},
			{Role: ai.RoleUser, Content: fmt.Sprintf("主题：%s。请用%s生成。", theme, lang)},
		},
		Temperature: 0.8,
		MaxTokens:   500,
	})
	if err != nil {
		return nil, fmt.Errorf("generate destinations: %w", err)
	}
	return parseDestinations(res.Text), nil
}

// parseDestinations 与 parseChips 同款容错解析。
func parseDestinations(text string) []Destination {
	text = strings.TrimSpace(text)
	if d, ok := tryParseDests(text); ok {
		return d
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		if d, ok := tryParseDests(text[start : end+1]); ok {
			return d
		}
	}
	return nil
}

func tryParseDests(raw string) ([]Destination, bool) {
	var dests []Destination
	if err := json.Unmarshal([]byte(raw), &dests); err != nil {
		return nil, false
	}
	out := make([]Destination, 0, len(dests))
	for _, d := range dests {
		d.Name = strings.TrimSpace(d.Name)
		d.Blurb = strings.TrimSpace(d.Blurb)
		if d.Name == "" {
			continue
		}
		if len([]rune(d.Name)) > maxLabelLen {
			d.Name = string([]rune(d.Name)[:maxLabelLen])
		}
		if len([]rune(d.Blurb)) > maxSampleLen {
			d.Blurb = string([]rune(d.Blurb)[:maxSampleLen])
		}
		out = append(out, d)
	}
	if len(out) < 2 {
		return nil, false
	}
	if len(out) > chipCount {
		out = out[:chipCount]
	}
	return out, true
}

// generate calls the model and parses the JSON array tolerantly.
func (s *Service) generate(ctx context.Context, locale string) ([]Chip, error) {
	ctx, cancel := context.WithTimeout(ctx, genTimeout)
	defer cancel()

	lang := "中文"
	if strings.HasPrefix(locale, "en") {
		lang = "English"
	}
	res, err := s.provider.ChatOnce(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: fmt.Sprintf(systemPrompt, chipCount)},
			{Role: ai.RoleUser, Content: fmt.Sprintf("请用%s生成。", lang)},
		},
		Temperature: 0.9, // 灵感要多样性，温度调高
		MaxTokens:   800,
	})
	if err != nil {
		return nil, fmt.Errorf("generate chips: %w", err)
	}
	return parseChips(res.Text), nil
}

// parseChips tolerantly extracts the JSON array: direct unmarshal first, then
// a salvage slice from the first '[' to the last ']' (models love wrapping
// JSON in markdown fences despite instructions).
func parseChips(text string) []Chip {
	text = strings.TrimSpace(text)
	if chips, ok := tryParse(text); ok {
		return chips
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		if chips, ok := tryParse(text[start : end+1]); ok {
			return chips
		}
	}
	return nil
}

func tryParse(raw string) ([]Chip, bool) {
	var chips []Chip
	if err := json.Unmarshal([]byte(raw), &chips); err != nil {
		return nil, false
	}
	out := make([]Chip, 0, len(chips))
	for _, c := range chips {
		c.Label = strings.TrimSpace(c.Label)
		c.Sample = strings.TrimSpace(c.Sample)
		if c.Label == "" || c.Sample == "" {
			continue
		}
		if len([]rune(c.Label)) > maxLabelLen {
			c.Label = string([]rune(c.Label)[:maxLabelLen])
		}
		if len([]rune(c.Sample)) > maxSampleLen {
			c.Sample = string([]rune(c.Sample)[:maxSampleLen])
		}
		out = append(out, c)
	}
	// 至少 3 条才有意义，否则视为解析失败（前端回退静态标签）
	if len(out) < 3 {
		return nil, false
	}
	if len(out) > chipCount {
		out = out[:chipCount]
	}
	return out, true
}

// --- cache ---

func cacheKey(locale string) string { return "inspire:chips:" + locale }

func (s *Service) getCached(ctx context.Context, locale string) ([]Chip, bool) {
	if s.rdb != nil {
		raw, err := s.rdb.Get(ctx, cacheKey(locale)).Bytes()
		if err == nil {
			var chips []Chip
			if json.Unmarshal(raw, &chips) == nil && len(chips) > 0 {
				return chips, true
			}
		} else if !errors.Is(err, redis.Nil) {
			slog.Warn("inspire cache read failed", "error", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.local[locale]; ok && time.Now().Before(c.expiresAt) {
		return c.chips, true
	}
	return nil, false
}

func (s *Service) setCached(ctx context.Context, locale string, chips []Chip) {
	if len(chips) == 0 {
		return
	}
	if s.rdb != nil {
		if raw, err := json.Marshal(chips); err == nil {
			if err := s.rdb.Set(ctx, cacheKey(locale), raw, cacheTTL).Err(); err != nil {
				slog.Warn("inspire cache write failed", "error", err)
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.local[locale] = cachedChips{chips: chips, expiresAt: time.Now().Add(cacheTTL)}
}

// --- destination cache（按 locale|theme） ---

func destCacheKey(localeTheme string) string { return "inspire:dest:" + localeTheme }

func (s *Service) getCachedDest(ctx context.Context, key string) ([]Destination, bool) {
	if s.rdb != nil {
		raw, err := s.rdb.Get(ctx, destCacheKey(key)).Bytes()
		if err == nil {
			var dests []Destination
			if json.Unmarshal(raw, &dests) == nil && len(dests) > 0 {
				return dests, true
			}
		} else if !errors.Is(err, redis.Nil) {
			slog.Warn("inspire dest cache read failed", "error", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.localDests[key]; ok && time.Now().Before(c.expiresAt) {
		return c.dests, true
	}
	return nil, false
}

func (s *Service) setCachedDest(ctx context.Context, key string, dests []Destination) {
	if len(dests) == 0 {
		return
	}
	if s.rdb != nil {
		if raw, err := json.Marshal(dests); err == nil {
			if err := s.rdb.Set(ctx, destCacheKey(key), raw, cacheTTL).Err(); err != nil {
				slog.Warn("inspire dest cache write failed", "error", err)
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localDests[key] = cachedDests{dests: dests, expiresAt: time.Now().Add(cacheTTL)}
}
