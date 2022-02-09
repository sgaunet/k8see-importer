-- migrate:up

CREATE TABLE public.k8sevents
(
    firstEventTs timestamp with time zone,
    eventTs timestamp with time zone,
    name character varying(100) NOT NULL,
    namespace character varying(100) NOT NULL,
    reason text ,
    type character varying(100),
    message character varying(512)
);

CREATE INDEX firstEventTs_idx    ON public.k8sevents (firstEventTs) ;
CREATE INDEX EventTs_idx    ON public.k8sevents (EventTs) ;
CREATE INDEX name_idx    ON public.k8sevents (name) ;

-- migrate:down

DROP TABLE public.k8sevents;
