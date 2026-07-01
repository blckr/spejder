package db

import (
	"database/sql"

	"codeberg.org/blckr/spejder/internal/classify"
)

func InsertConnection(database *sql.DB, srcIP string, dstPort uint16, country, city string, asn uint, asnOrg string, t classify.Type) error {
	_, err := database.Exec(
		`INSERT INTO connections (src_ip, dst_port, country, city, asn, asn_org, traffic_type) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		srcIP, dstPort, country, city, asn, asnOrg, string(t),
	)
	return err
}
