CREATE TABLE companies (
    id INTEGER PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    spotify_url TEXT NOT NULL
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    display_name TEXT NOT NULL,
    company_code TEXT NOT NULL REFERENCES companies(code)
);

CREATE TABLE memberships (
    organization_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE labels (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);
