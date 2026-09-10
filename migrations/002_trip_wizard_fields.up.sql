-- 002: 补齐 PRD 创建向导字段（destination/travelers_count/偏好标签/自然语言需求）
ALTER TABLE trips
    ADD COLUMN destination     text,
    ADD COLUMN travelers_count int CHECK (travelers_count IS NULL OR travelers_count BETWEEN 1 AND 100);

ALTER TABLE trip_preferences
    ADD COLUMN preferences      jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN natural_language text;
