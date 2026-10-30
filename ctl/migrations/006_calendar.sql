-- Personal calendar: events and tasks with optional reminders, plus a monthly planner page
-- (goals, important tasks, notes) like a paper planner.
CREATE TABLE cal_events (
    id             bigserial PRIMARY KEY,
    user_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title          text NOT NULL,
    notes          text NOT NULL DEFAULT '',
    starts_at      timestamptz NOT NULL,
    ends_at        timestamptz,
    all_day        boolean NOT NULL DEFAULT false,
    color          text NOT NULL DEFAULT 'blue',
    is_task        boolean NOT NULL DEFAULT false,
    done           boolean NOT NULL DEFAULT false,
    remind_minutes integer,            -- NULL = no reminder; minutes before the start
    reminded_at    timestamptz,
    mail_ref       text NOT NULL DEFAULT '', -- "account|folder|uid" when created from a letter
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cal_events_user_time ON cal_events (user_id, starts_at);
CREATE INDEX cal_events_due ON cal_events (starts_at) WHERE remind_minutes IS NOT NULL AND reminded_at IS NULL;

CREATE TABLE cal_months (
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month      date NOT NULL, -- first day of the month
    goals      jsonb NOT NULL DEFAULT '[]',
    tasks      jsonb NOT NULL DEFAULT '[]',
    notes      text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, month)
);
