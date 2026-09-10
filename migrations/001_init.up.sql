-- TripWeave 001_init: 全部核心表（按 docs/DATABASE.md）
-- ID 策略: uuid + gen_random_uuid()（PG13+ 内置）
-- 枚举策略: TEXT + CHECK（后续改枚举不用动类型）
-- 删除策略: Trip→Day→Activity 级联；Location/Hotel/Restaurant 为共享数据不级联

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    password_hash text NOT NULL,
    display_name  text NOT NULL,
    avatar_url    text,
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','deleted')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_uidx ON users (lower(email));

CREATE TABLE user_preferences (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    preferences jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id)
);

CREATE TABLE trips (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      text NOT NULL,
    start_date date,
    end_date   date,
    status     text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','planning','ready','archived','deleted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (start_date IS NULL OR end_date IS NULL OR end_date >= start_date)
);
CREATE INDEX trips_owner_id_idx ON trips (owner_id);

CREATE TABLE trip_members (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       text NOT NULL DEFAULT 'viewer' CHECK (role IN ('owner','editor','viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trip_id, user_id)
);
CREATE INDEX trip_members_trip_id_idx ON trip_members (trip_id);
CREATE INDEX trip_members_user_id_idx ON trip_members (user_id);

CREATE TABLE trip_preferences (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id        uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    budget         numeric(12,2) CHECK (budget IS NULL OR budget >= 0),
    transport_mode text,
    travel_style   text,
    constraints    jsonb NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trip_id)
);

CREATE TABLE trip_days (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    day_number int NOT NULL CHECK (day_number > 0),
    date       date,
    title      text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trip_id, day_number)
);

CREATE TABLE locations (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          text NOT NULL,
    provider_place_id text NOT NULL,
    name              text NOT NULL,
    latitude          double precision,
    longitude         double precision,
    address           text,
    city              text,
    country           text,
    timezone          text,
    metadata          jsonb NOT NULL DEFAULT '{}',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_place_id)
);

CREATE TABLE activities (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_day_id uuid NOT NULL REFERENCES trip_days(id) ON DELETE CASCADE,
    type        text NOT NULL DEFAULT 'other' CHECK (type IN ('attraction','restaurant','cafe','hotel','transport','free_time','other')),
    title       text NOT NULL,
    location_id uuid REFERENCES locations(id) ON DELETE SET NULL,
    start_time  time,
    end_time    time,
    sort_order  int NOT NULL DEFAULT 0,
    notes       text,
    status      text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','done','skipped')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activities_day_sort_idx ON activities (trip_day_id, sort_order);

CREATE TABLE hotels (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text,
    price_level int CHECK (price_level IS NULL OR price_level BETWEEN 1 AND 5),
    rating      numeric(2,1) CHECK (rating IS NULL OR rating BETWEEN 0 AND 5),
    metadata    jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE restaurants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name        text NOT NULL,
    cuisine     text,
    price_level int CHECK (price_level IS NULL OR price_level BETWEEN 1 AND 5),
    rating      numeric(2,1) CHECK (rating IS NULL OR rating BETWEEN 0 AND 5),
    metadata    jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ai_conversations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    trip_id    uuid REFERENCES trips(id) ON DELETE CASCADE,
    title      text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_conversations_user_id_idx ON ai_conversations (user_id);
CREATE INDEX ai_conversations_trip_id_idx ON ai_conversations (trip_id);

CREATE TABLE ai_messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('system','user','assistant','tool')),
    content         text NOT NULL,
    model           text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_messages_conv_created_idx ON ai_messages (conversation_id, created_at);

CREATE TABLE ai_tool_calls (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id uuid NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
    tool_name  text NOT NULL,
    arguments  jsonb,
    result     jsonb,
    status     text CHECK (status IN ('success','error')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE trip_generation_tasks (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id        uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','cancelled')),
    provider       text,
    model          text,
    prompt_version text,
    error          text,
    started_at     timestamptz,
    completed_at   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE trip_shares (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    token      text NOT NULL UNIQUE,
    permission text NOT NULL DEFAULT 'viewer' CHECK (permission IN ('viewer','editor')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
