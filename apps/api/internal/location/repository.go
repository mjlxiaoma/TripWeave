package location

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Location is a stored places-table row (the subset exposed to API clients).
// Provider metadata stays internal; clients only need identity + coordinates.
type Location struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Address   *string  `json:"address"`
	City      *string  `json:"city"`
}

// Repository persists locations (deduplicated by provider + provider_place_id).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a location repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Upsert inserts or refreshes a location keyed by (provider, provider_place_id)
// and returns the stored row with its uuid. Concurrent upserts of the same
// place are safe: the unique constraint absorbs the race.
func (r *Repository) Upsert(ctx context.Context, provider, placeID string, p POI) (*Location, error) {
	var loc Location
	err := r.pool.QueryRow(ctx, `
		INSERT INTO locations (provider, provider_place_id, name, latitude, longitude, address, city)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (provider, provider_place_id) DO UPDATE SET
			name       = EXCLUDED.name,
			latitude   = EXCLUDED.latitude,
			longitude  = EXCLUDED.longitude,
			address    = COALESCE(EXCLUDED.address, locations.address),
			city       = COALESCE(EXCLUDED.city, locations.city),
			updated_at = now()
		RETURNING id, name, latitude, longitude, address, city`,
		provider, placeID, p.Name, p.Latitude, p.Longitude, nilIfEmpty(p.Address), nilIfEmpty(p.City),
	).Scan(&loc.ID, &loc.Name, &loc.Latitude, &loc.Longitude, &loc.Address, &loc.City)
	if err != nil {
		return nil, err
	}
	return &loc, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
