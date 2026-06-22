// pkg/asn/asn.go
package asn

// Lookup resolves an IP to its ASN/owner from an offline dataset.
// Phase 0 stub; WS-I implements Load + Lookup against an ip2asn TSV.
type Lookup struct{}

func Load(path string) (*Lookup, error) { return &Lookup{}, nil }

func (l *Lookup) Lookup(ip string) (asn uint32, org string, ok bool) { return 0, "", false }
