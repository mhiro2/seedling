CREATE TABLE companies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies (id),
    name TEXT NOT NULL
);

CREATE TABLE projects (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies (id),
    name TEXT NOT NULL
);

CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects (id),
    assignee_user_id BIGINT NOT NULL REFERENCES users (id),
    title TEXT NOT NULL,
    status TEXT NOT NULL
);

CREATE TABLE departments (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE employees (
    id BIGSERIAL PRIMARY KEY,
    department_id BIGINT NOT NULL REFERENCES departments (id),
    name TEXT NOT NULL
);

CREATE TABLE regions (
    code TEXT NOT NULL,
    number BIGINT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (code, number)
);

CREATE TABLE deployments (
    id BIGSERIAL PRIMARY KEY,
    region_code TEXT NOT NULL,
    region_number BIGINT NOT NULL,
    name TEXT NOT NULL,
    FOREIGN KEY (region_code, region_number) REFERENCES regions (code, number)
);

CREATE TABLE articles (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL
);

CREATE TABLE tags (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE article_tags (
    article_id BIGINT NOT NULL REFERENCES articles (id),
    tag_id BIGINT NOT NULL REFERENCES tags (id),
    PRIMARY KEY (article_id, tag_id)
);
