ALTER TABLE trip_preferences
    DROP COLUMN IF EXISTS natural_language,
    DROP COLUMN IF EXISTS preferences;

ALTER TABLE trips
    DROP COLUMN IF EXISTS travelers_count,
    DROP COLUMN IF EXISTS destination;
