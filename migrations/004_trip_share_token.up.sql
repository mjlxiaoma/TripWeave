-- 行程分享：share_token 作为公开只读链接的秘密（不可枚举，撤销即清空）。
ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS share_token text;

CREATE UNIQUE INDEX IF NOT EXISTS trips_share_token_uq
    ON trips (share_token)
    WHERE share_token IS NOT NULL;
