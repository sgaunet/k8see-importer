-- migrate:up

ALTER TABLE public.k8sevents ALTER COLUMN message type character varying(2048);

-- migrate:down

ALTER TABLE public.k8sevents ALTER COLUMN message type character varying(512);
