DROP INDEX IF EXISTS trips_share_token_uq;

ALTER TABLE trips
    DROP COLUMN IF EXISTS share_token;
