-- +goose Up

-- Add BIGSERIAL primary key column
ALTER TABLE k8sevents ADD COLUMN id BIGSERIAL PRIMARY KEY;

-- Remove duplicates before adding unique constraint
-- Keep the most recent (highest ctid) entry for each duplicate group
DELETE FROM k8sevents a USING k8sevents b
WHERE a.ctid < b.ctid
  AND a.name = b.name
  AND a.namespace = b.namespace
  AND a.eventTime = b.eventTime
  AND a.firstTime = b.firstTime;

-- Create unique index to prevent duplicate events
CREATE UNIQUE INDEX k8sevents_unique_event_idx
ON k8sevents (name, namespace, eventTime, firstTime);

-- +goose Down

DROP INDEX k8sevents_unique_event_idx;
ALTER TABLE k8sevents DROP COLUMN id;
