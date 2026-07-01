package classify

import (
	"net"
	"strings"
)

type Type string

const (
	Internal Type = "internal" // private/loopback address — your own infrastructure
	Scanner  Type = "scanner"  // probing unusual ports — always automated
	Bot      Type = "bot"      // datacenter ASN hitting common ports
	Unknown  Type = "unknown"  // residential/unknown ASN on common ports
)

// commonPorts are ports that real services might legitimately expose.
// Anything outside this list hitting your server is a scanner.
var commonPorts = map[uint16]bool{
	22: true, 25: true, 80: true, 443: true,
	587: true, 465: true, 8080: true, 8443: true,
}

// datacenterKeywords matches ASN orgs that are hosting/cloud providers.
var datacenterKeywords = []string{
	"hetzner", "ovh", "contabo", "leaseweb", "digitalocean", "linode",
	"vultr", "scaleway", "amazon", "google", "microsoft", "alibaba",
	"tencent", "cloudflare", "oracle", "choopa", "datacamp", "frantech",
	"m247", "serverius",
}

var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "::1/128", "fc00::/7", "169.254.0.0/16",
	} {
		_, network, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, network)
	}
}

func Classify(srcIP net.IP, dstPort uint16, asnOrg string) Type {
	for _, network := range privateRanges {
		if network.Contains(srcIP) {
			return Internal
		}
	}
	if !commonPorts[dstPort] {
		return Scanner
	}
	if isDatacenter(asnOrg) {
		return Bot
	}
	return Unknown
}

func isDatacenter(asnOrg string) bool {
	lower := strings.ToLower(asnOrg)
	for _, kw := range datacenterKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
