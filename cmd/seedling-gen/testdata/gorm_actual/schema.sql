CREATE TABLE countries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    display_name TEXT NOT NULL,
    nickname TEXT,
    country_code TEXT NOT NULL,
    FOREIGN KEY (country_code) REFERENCES countries(code)
);

CREATE TABLE memberships (
    organization_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    PRIMARY KEY (organization_id, user_id)
);
