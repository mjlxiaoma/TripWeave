-- TripWeave 005_trip_cover: 旅行自定义封面。
-- 独立表而非 trips 列:列表查询不拖图片流量,GET 端点按需返回并缓存。
-- 随旅行删除级联清理。
CREATE TABLE trip_covers (
    trip_id      uuid PRIMARY KEY REFERENCES trips(id) ON DELETE CASCADE,
    image        bytea NOT NULL,
    content_type text  NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
