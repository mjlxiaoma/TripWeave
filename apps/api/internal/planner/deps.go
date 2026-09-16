package planner

import (
	"context"
	"encoding/json"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/location"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
)

// Narrow interfaces defined at the consumer (planner) so the engine and tools
// are unit-testable with fakes. The concrete trip/day/planner repositories
// satisfy these without any change to their packages.

// TripStore is the trip persistence the planner needs.
type TripStore interface {
	Get(ctx context.Context, tripID, userID string) (*trip.Detail, error)
	Role(ctx context.Context, tripID, userID string) (string, error)
	Update(ctx context.Context, tripID string, p trip.Patch) (*trip.Trip, *trip.Preference, error)
}

// DayStore is the day/activity persistence the planner needs.
type DayStore interface {
	ListDays(ctx context.Context, tripID string) ([]day.Day, error)
	CreateItinerary(ctx context.Context, tripID string, days []day.ItineraryDayInput) ([]day.Day, error)
	CreateDay(ctx context.Context, tripID string, in day.DayInput) (*day.Day, error)
	CreateActivity(ctx context.Context, dayID string, in day.ActivityInput) (*day.Activity, error)
	UpdateActivity(ctx context.Context, activityID string, p day.ActivityPatch) (*day.Activity, error)
	DeleteActivity(ctx context.Context, activityID string) error
	ReorderActivities(ctx context.Context, dayID string, ids []string) error
	TripIDForActivity(ctx context.Context, activityID string) (string, error)
	GetActivity(ctx context.Context, activityID string) (*day.Activity, error)
}

// Locator resolves an activity title to a map location (nil-safe: the engine
// treats a nil Locator or a nil result as "not located" and keeps going).
type Locator interface {
	Locate(ctx context.Context, title, city string) (*location.Location, error)
}

// Store is the planner's own persistence (conversations/messages/tool calls).
type Store interface {
	GetOrCreateConversation(ctx context.Context, tripID, userID string) (*Conversation, error)
	AppendMessage(ctx context.Context, conversationID, role, content, model string) (string, error)
	RecordToolCall(ctx context.Context, messageID, name string, args json.RawMessage) (string, error)
	FinishToolCall(ctx context.Context, id, status string, result json.RawMessage) error
	CreateTask(ctx context.Context, tripID, provider, model, promptVersion string) (string, error)
	FinishTask(ctx context.Context, id, status, errMsg string) error
	ListRecentMessages(ctx context.Context, conversationID string, limit int) ([]HistoryMessage, error)
}

// Lock is an acquired per-trip generation lock.
type Lock interface {
	Release(ctx context.Context)
}

// Locker acquires per-trip generation locks.
type Locker interface {
	Acquire(ctx context.Context, tripID string) (Lock, error)
}
