package db

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

type IPDetail struct {
	IP       string
	Port     int
	Country  string
	ASNOrg   string
	SeenAt   string
	Duration string
}

type DrillItem struct {
	Label string
	Count int
	Key   string
}

func QueryDrill(database *sql.DB, groupCol string, whereClause string, args []any, limit int) ([]DrillItem, error) {
	fullWhere := "direction = 'incoming'"
	if whereClause != "" {
		fullWhere += " AND " + whereClause
	}

	query := fmt.Sprintf(`
			SELECT %s, COUNT(*) as cnt
			FROM connections
			WHERE %s
			GROUP BY %s
			ORDER BY cnt DESC
			LIMIT ?
		`, groupCol, fullWhere, groupCol)

	queryArgs := append(args, limit)
	rows, err := database.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DrillItem
	for rows.Next() {
		var item DrillItem
		if err := rows.Scan(&item.Label, &item.Count); err != nil {
			return nil, err
		}
		item.Key = item.Label
		if item.Label == "" {
			if groupCol == "city" {
				item.Label = "???"
			} else if groupCol == "country" {
				item.Label = "??"
			} else {
				item.Label = "unknown"
			}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Lowest Level connection
func QueryLeaf(database *sql.DB, whereClause string, args []any, limit int) ([]IPDetail, error) {
	fullWhere := "direction = 'incoming'"
	if whereClause != "" {
		fullWhere += " AND " + whereClause
	}

	query := fmt.Sprintf(`
			SELECT src_ip, dst_port, country, asn_org, seen_at, IFNULL(duration_ms, 0)
			FROM connections
			WHERE %s
			ORDER BY seen_at DESC
			LIMIT ?
		`, fullWhere)

	queryArgs := append(args, limit)
	rows, err := database.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []IPDetail
	for rows.Next() {
		var d IPDetail
		if err := rows.Scan(&d.IP, &d.Port, &d.Country, &d.ASNOrg, &d.SeenAt, &d.Duration); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// TimeFilter represents a time window.
type TimeFilter int

const (
	FilterAll TimeFilter = iota
	Filter6Months
	Filter3Months
	Filter1Month
	Filter1Week
	Filter3Days
	Filter1Day
	Filter1Hour
)

func (f TimeFilter) Where() string {
	switch f {
	case Filter6Months:
		return "seen_at >= datetime('now', '-6 month')"
	case Filter3Months:
		return "seen_at >= datetime('now', '-3 month')"
	case Filter1Month:
		return "seen_at >= datetime('now', '-1 month')"
	case Filter1Week:
		return "seen_at >= datetime('now', '-7 days')"
	case Filter3Days:
		return "seen_at >= datetime('now', '-3 days')"
	case Filter1Day:
		return "seen_at >= datetime('now', '-1 day')"
	case Filter1Hour:
		return "seen_at >= datetime('now', '-1 hour')"
	default:
		return "1=1"
	}
}

type DayCount struct {
	Label string
	Count int
}

func (f TimeFilter) Label() string {
	return []string{"All Time", "6 Months", "3 Months", "1 Month", "1 Week", "3 Days", "1 Day", "1 Hour"}[f]
}

type Filter struct {
	Time TimeFilter
	Port uint16 // 0 = all ports
}

func (f Filter) Where() string {
	w := f.Time.Where()
	if f.Port != 0 {
		w += fmt.Sprintf(" AND dst_port = %d", f.Port)
	}
	return w
}

// IPSummary returns aggregated connection info for a specific source IP.
type IPSummary struct {
	IP          string
	Country     string
	City        string
	ASN         int
	ASNOrg      string
	TrafficType string
	LastSeen    time.Time
	Total       int
}

func IPSummaryForIP(database *sql.DB, ip string) (IPSummary, error) {
	const q = `
		SELECT src_ip, country, city, asn, asn_org, traffic_type, max(seen_at), count(*)
		FROM connections
		WHERE src_ip = ?`

	var s IPSummary
	var lastSeen string
	err := database.QueryRow(q, ip).Scan(
		&s.IP, &s.Country, &s.City, &s.ASN, &s.ASNOrg, &s.TrafficType, &lastSeen, &s.Total,
	)
	if err != nil {
		return s, err
	}
	s.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
	return s, nil
}

// TopPorts retrieves the most targeted ports for the Ports panel.
func TopPorts(database *sql.DB, limit int) ([]DrillItem, error) {
	rows, err := database.Query(`
		SELECT dst_port, COUNT(*) as cnt
		FROM connections
		WHERE direction = 'incoming'
		GROUP BY dst_port
		ORDER BY cnt DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DrillItem
	for rows.Next() {
		var port int
		var count int
		if err := rows.Scan(&port, &count); err != nil {
			return nil, err
		}
		label := fmt.Sprintf("Port %d", port)
		result = append(result, DrillItem{Label: label, Count: count, Key: strconv.Itoa(port)})
	}
	return result, nil
}

// ConnectionDurations groups connection counts by their duration.
func ConnectionDurations(database *sql.DB) ([]DrillItem, error) {
	q := `
		SELECT
			SUM(CASE WHEN closed_at IS NULL THEN 1 ELSE 0 END) as active,
			SUM(CASE WHEN duration_ms IS NOT NULL AND duration_ms < 1000 THEN 1 ELSE 0 END) as instant,
			SUM(CASE WHEN duration_ms >= 1000 AND duration_ms < 60000 THEN 1 ELSE 0 END) as short,
			SUM(CASE WHEN duration_ms >= 60000 AND duration_ms < 600000 THEN 1 ELSE 0 END) as medium,
			SUM(CASE WHEN duration_ms >= 600000 AND duration_ms < 3600000 THEN 1 ELSE 0 END) as long,
			SUM(CASE WHEN duration_ms >= 3600000 THEN 1 ELSE 0 END) as persistent
		FROM connections
		WHERE direction = 'incoming'`

	row := database.QueryRow(q)
	var active, instant, short, medium, long, persistent int
	if err := row.Scan(&active, &instant, &short, &medium, &long, &persistent); err != nil {
		return nil, err
	}

	return []DrillItem{
		{Label: "Still Active", Count: active, Key: "active"},
		{Label: "Instant (<1s)", Count: instant, Key: "instant"},
		{Label: "Short (1s-1m)", Count: short, Key: "short"},
		{Label: "Medium (1m-10m)", Count: medium, Key: "medium"},
		{Label: "Long (10m-1h)", Count: long, Key: "long"},
		{Label: "Persistent (>1h)", Count: persistent, Key: "persistent"},
	}, nil
}


