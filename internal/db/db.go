package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS connections (
    id           INTEGER PRIMARY KEY,
    direction    TEXT    NOT NULL DEFAULT 'incoming',
    src_ip       TEXT    NOT NULL,
    src_port     INTEGER NOT NULL DEFAULT 0,
    dst_port     INTEGER NOT NULL,
    country      TEXT    NOT NULL DEFAULT '',
    city         TEXT    NOT NULL DEFAULT '',
    asn          INTEGER NOT NULL DEFAULT 0,
    asn_org      TEXT    NOT NULL DEFAULT '',
    traffic_type TEXT    NOT NULL DEFAULT 'unknown',
    seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at    DATETIME,
    duration_ms  INTEGER,
    close_reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_seen_at      ON connections (seen_at);
CREATE INDEX IF NOT EXISTS idx_dst_port     ON connections (dst_port);
CREATE INDEX IF NOT EXISTS idx_country      ON connections (country);
CREATE INDEX IF NOT EXISTS idx_asn_org      ON connections (asn_org);
CREATE INDEX IF NOT EXISTS idx_traffic_type ON connections (traffic_type);
CREATE INDEX IF NOT EXISTS idx_direction    ON connections (direction);

`

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return db, nil
}
