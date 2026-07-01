package geo

import (
	"net"

	"github.com/oschwald/geoip2-golang"
)

type Info struct {
	Country string // "DE", "US", "CN"
	City    string // "Berlin", "New York"
	ASN     uint   // 24940
	ASNOrg  string // "Hetzner Online GmbH"
}

type Reader struct {
	city *geoip2.Reader
	asn  *geoip2.Reader
}

func Open(cityPath, asnPath string) (*Reader, error) {
	city, err := geoip2.Open(cityPath)
	if err != nil {
		return nil, err
	}

	asn, err := geoip2.Open(asnPath)
	if err != nil {
		city.Close()
		return nil, err
	}

	return &Reader{city: city, asn: asn}, nil
}

func (r *Reader) Lookup(ip net.IP) Info {
	info := Info{}

	if record, err := r.city.City(ip); err == nil {
		info.Country = record.Country.IsoCode
		info.City = record.City.Names["en"]
	}

	if record, err := r.asn.ASN(ip); err == nil {
		info.ASN = uint(record.AutonomousSystemNumber)
		info.ASNOrg = record.AutonomousSystemOrganization
	}

	return info
}

func (r *Reader) Close() {
	r.city.Close()
	r.asn.Close()
}
