-- TripWeave 003_email_verification: 邮箱验证
-- 已注册的老账号视为已验证（回填 created_at），避免上线后老用户被锁死

ALTER TABLE users ADD COLUMN email_verified_at timestamptz;
UPDATE users SET email_verified_at = created_at;
