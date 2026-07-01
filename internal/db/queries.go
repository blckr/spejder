package db

import "database/sql"

func InsertConnection(db *sql.DB, srcIP string, dstPort uint16, country string) error {
	_, err := db.Exec(
		`INSERT INTO connections (src_ip, dst_port, country) VALUES (?, ?, ?)`,
		srcIP, dstPort, country,
	)
	return err
}
