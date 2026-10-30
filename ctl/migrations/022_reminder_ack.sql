-- Reminders repeat every 5 minutes until the user confirms them ("Done") or snoozes them.
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS ack_at timestamptz;
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS snooze_until timestamptz;
