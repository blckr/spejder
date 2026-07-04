package db

import (
	"database/sql"

	"codeberg.org/blckr/spejder/internal/classify"
)

func InsertConnection(
	database *sql.DB,
	direction string,
	remoteIP string,
	remotePort uint16,
	localPort uint16,
	country, city string,
	asn uint,
	asnOrg string,
	t classify.Type,
) (int64, error) {
	res, err := database.Exec(
		`INSERT INTO connections (direction, src_ip, src_port, dst_port, country, city, asn, asn_org, traffic_type)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		direction, remoteIP, remotePort, localPort, country, city, asn, asnOrg, string(t),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func CloseConnection(database *sql.DB, id int64, durationMs int64, closeReason string) error {
	_, err := database.Exec(
		`UPDATE connections
		 SET closed_at = CURRENT_TIMESTAMP, duration_ms = ?, close_reason = ?
		 WHERE id = ?`,
		durationMs, closeReason, id,
	)
	return err
}
