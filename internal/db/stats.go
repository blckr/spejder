package db

import (
	"database/sql"
	"fmt"
	"time"
)

type TimeFilter int

const (
	FilterAll   TimeFilter = iota
	FilterMonth
	FilterWeek
	Filter3Days
	Filter24h
)

func (f TimeFilter) where() string {
	switch f {
	case FilterMonth:
		return "seen_at >= datetime('now', '-1 month')"
	case FilterWeek:
		return "seen_at >= datetime('now', '-7 days')"
	case Filter3Days:
		return "seen_at >= datetime('now', '-3 days')"
	case Filter24h:
		return "seen_at >= datetime('now', '-1 day')"
	default:
		return "1=1"
	}
}

func (f TimeFilter) Label() string {
	return []string{"All Time", "Month", "Week", "3 Days", "24h"}[f]
}

// Filter combines a time window with an optional port constraint.
type Filter struct {
	Time TimeFilter
	Port uint16 // 0 = all ports
}

func (f Filter) where() string {
	w := f.Time.where()
	if f.Port != 0 {
		w += fmt.Sprintf(" AND dst_port = %d", f.Port)
	}
	return w
}

type CountryCount struct {
	Country string
	Count   int
}

type DayCount struct {
	Label string
	Count int
}

type TrafficStats struct {
	Internal int
	Scanner  int
	Bot      int
	Unknown  int
	Total    int
}


func TopCountries(database *sql.DB, f Filter, limit int) ([]CountryCount, error) {
	q := fmt.Sprintf(`
		SELECT country, count(*) as cnt
		FROM connections
		WHERE country != '' AND %s
		GROUP BY country
		ORDER BY cnt DESC
		LIMIT ?`, f.where())

	rows, err := database.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CountryCount
	for rows.Next() {
		var c CountryCount
		if err := rows.Scan(&c.Country, &c.Count); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func TrafficByType(database *sql.DB, f Filter) (TrafficStats, error) {
	q := fmt.Sprintf(`
		SELECT
			SUM(CASE WHEN traffic_type = 'internal' THEN 1 ELSE 0 END),
			SUM(CASE WHEN traffic_type = 'scanner'  THEN 1 ELSE 0 END),
			SUM(CASE WHEN traffic_type = 'bot'      THEN 1 ELSE 0 END),
			SUM(CASE WHEN traffic_type = 'unknown'  THEN 1 ELSE 0 END),
			COUNT(*)
		FROM connections WHERE %s`, f.where())

	var s TrafficStats
	err := database.QueryRow(q).Scan(&s.Internal, &s.Scanner, &s.Bot, &s.Unknown, &s.Total)
	return s, err
}

func DailyConnections(database *sql.DB, f Filter) ([]DayCount, error) {
	label := "date(seen_at)"
	if f.Time == Filter24h {
		label = "strftime('%H:00', seen_at)"
	}
	q := fmt.Sprintf(`
		SELECT %s, count(*) FROM connections
		WHERE %s GROUP BY 1 ORDER BY 1 ASC`, label, f.where())

	rows, err := database.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DayCount
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Label, &d.Count); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// TimeSeries returns connection counts bucketed by time with granularity depending on the filter.
// 24h → hourly, 3days → 4h, week → 8h, month/all → daily.
func TimeSeries(database *sql.DB, f Filter) ([]DayCount, error) {
	var label string
	switch f.Time {
	case Filter24h:
		label = "strftime('%Y-%m-%d %H:00', seen_at)"
	case Filter3Days:
		label = "strftime('%Y-%m-%d ', seen_at) || printf('%02d:00', (cast(strftime('%H', seen_at) as integer) / 4) * 4)"
	case FilterWeek:
		label = "strftime('%Y-%m-%d ', seen_at) || printf('%02d:00', (cast(strftime('%H', seen_at) as integer) / 8) * 8)"
	default:
		label = "date(seen_at)"
	}
	q := fmt.Sprintf(`
		SELECT %s, count(*) FROM connections
		WHERE %s GROUP BY 1 ORDER BY 1 ASC`, label, f.where())

	rows, err := database.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DayCount
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Label, &d.Count); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func TopCitiesForCountry(database *sql.DB, f Filter, country string, limit int) ([]CountryCount, error) {
	q := fmt.Sprintf(`
		SELECT city, count(*) as cnt FROM connections
		WHERE country = ? AND %s
		GROUP BY city ORDER BY cnt DESC LIMIT ?`, f.where())

	rows, err := database.Query(q, country, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CountryCount
	for rows.Next() {
		var c CountryCount
		if err := rows.Scan(&c.Country, &c.Count); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func TopASNsForCountryCity(database *sql.DB, f Filter, country, city string, limit int) ([]CountryCount, error) {
	q := fmt.Sprintf(`
		SELECT asn_org, count(*) as cnt FROM connections
		WHERE country = ? AND city = ? AND %s
		GROUP BY asn_org ORDER BY cnt DESC LIMIT ?`, f.where())

	rows, err := database.Query(q, country, city, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CountryCount
	for rows.Next() {
		var c CountryCount
		if err := rows.Scan(&c.Country, &c.Count); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func TopASNsForType(database *sql.DB, f Filter, trafficType string, limit int) ([]CountryCount, error) {
	q := fmt.Sprintf(`
		SELECT asn_org, count(*) as cnt FROM connections
		WHERE traffic_type = ? AND %s
		GROUP BY asn_org ORDER BY cnt DESC LIMIT ?`, f.where())

	rows, err := database.Query(q, trafficType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CountryCount
	for rows.Next() {
		var c CountryCount
		if err := rows.Scan(&c.Country, &c.Count); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

type IPDetail struct {
	IP      string
	Port    int
	Country string
	ASNOrg  string
	SeenAt  string
}

func IPsForASN(database *sql.DB, f Filter, asnOrg string, limit int) ([]IPDetail, error) {
	q := fmt.Sprintf(`
		SELECT src_ip, dst_port, country, asn_org, seen_at FROM connections
		WHERE asn_org = ? AND %s
		ORDER BY seen_at DESC LIMIT ?`, f.where())

	rows, err := database.Query(q, asnOrg, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []IPDetail
	for rows.Next() {
		var d IPDetail
		if err := rows.Scan(&d.IP, &d.Port, &d.Country, &d.ASNOrg, &d.SeenAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

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

// IPSummaryForIP returns aggregated connection info for a specific source IP.
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
