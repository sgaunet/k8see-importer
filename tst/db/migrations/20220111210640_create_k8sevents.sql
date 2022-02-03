-- migrate:up

CREATE TABLE public.k8sevents
(
    date timestamp with time zone NOT NULL,
    name character varying(100) NOT NULL,
    reason text ,
    type character varying(100),
    message character varying(512)
);

CREATE INDEX date_idx    ON public.k8sevents (date) ;
CREATE INDEX name_idx    ON public.k8sevents (name) ;

-- migrate:down

DROP TABLE public.k8sevents;
