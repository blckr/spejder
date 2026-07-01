package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS connections (
    id        INTEGER PRIMARY KEY,
    src_ip    TEXT    NOT NULL,
    dst_port  INTEGER NOT NULL,
    country   TEXT    NOT NULL DEFAULT '',
    seen_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_seen_at  ON connections (seen_at);
CREATE INDEX IF NOT EXISTS idx_dst_port ON connections (dst_port);
CREATE INDEX IF NOT EXISTS idx_country  ON connections (country);
`

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return db, nil
}
