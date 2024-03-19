-- +goose Up

ALTER TABLE k8sevents ALTER COLUMN message type character varying(2048);

-- +goose Down

ALTER TABLE k8sevents ALTER COLUMN message type character varying(512);
