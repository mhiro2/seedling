CREATE TABLE public.companies (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL
);

CREATE TABLE public.users (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    CONSTRAINT users_company_id_fkey
        FOREIGN KEY (company_id)
        REFERENCES public.companies(id)
