-- migrate:up

CREATE TABLE public.k8sevents
(
    exportedTime timestamp with time zone,
    firstTime timestamp with time zone,
    eventTime timestamp with time zone,
    name character varying(100) NOT NULL,
    namespace character varying(100) NOT NULL,
    reason text ,
    type character varying(100),
    message character varying(512)
);

CREATE INDEX exportedTime_idx    ON public.k8sevents (exportedTime) ;
CREATE INDEX firstTime_idx    ON public.k8sevents (firstTime) ;
CREATE INDEX eventTime_idx    ON public.k8sevents (eventTime) ;
CREATE INDEX name_idx    ON public.k8sevents (name) ;

-- migrate:down

DROP TABLE public.k8sevents;
