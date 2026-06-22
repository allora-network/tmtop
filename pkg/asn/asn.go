// pkg/asn/asn.go — offline ASN lookup backed by an ip2asn-v4.tsv dataset.
//
// Dataset source: https://iptoasn.com/ (public-domain, updated daily).
// Column layout (tab-separated):
//
//	range_start  range_end  AS_number  country_code  AS_description
//
// IPs are dotted-quad strings.  Load parses the file into a sorted slice of
// ranges; Lookup finds the candidate range with sort.Search and checks
// containment — O(log n) per query.
package asn

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

// asnRange holds one row from the TSV.
type asnRange struct {
	start uint32
	end   uint32
	asn   uint32
	org   string
}

// Lookup resolves an IP to its ASN/owner from an offline dataset.
type Lookup struct {
	ranges []asnRange // sorted by start, ascending
}

// Load parses path (ip2asn-v4.tsv format) and returns a ready Lookup.
// Malformed lines are silently skipped; the file must exist.
func Load(path string) (*Lookup, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("asn.Load: open %q: %w", path, err)
	}
	defer f.Close()

	var ranges []asnRange
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue // skip malformed
		}
		start, err := ipToU32(fields[0])
		if err != nil {
			continue
		}
		end, err := ipToU32(fields[1])
		if err != nil {
			continue
		}
		asnNum, err := strconv.ParseUint(fields[2], 10, 32)
		if err != nil {
			continue
		}
		// fields[3] = country_code (not stored), fields[4] = description
		org := fields[4]
		// ip2asn uses "Not routed" for unannounced space; include anyway
		ranges = append(ranges, asnRange{
			start: start,
			end:   end,
			asn:   uint32(asnNum),
			org:   org,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("asn.Load: scan %q: %w", path, err)
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	return &Lookup{ranges: ranges}, nil
}

// Lookup resolves ip to its AS number and organisation description.
// Returns (0, "", false) on any parse error or when ip falls outside all
// known ranges. Safe to call on a nil *Lookup (returns the zero tuple).
func (l *Lookup) Lookup(ip string) (asn uint32, org string, ok bool) {
	if l == nil || len(l.ranges) == 0 {
		return 0, "", false
	}
	addr, err := ipToU32(ip)
	if err != nil {
		return 0, "", false
	}

	// Find the rightmost range whose start <= addr.
	// sort.Search returns the smallest index i where f(i) is true.
	// We want the largest i where ranges[i].start <= addr, so we search for
	// the first index where start > addr and step back one.
	n := len(l.ranges)
	i := sort.Search(n, func(k int) bool {
		return l.ranges[k].start > addr
	})
	// i is now the first range with start > addr.
	// The candidate is i-1 (if it exists).
	if i == 0 {
		return 0, "", false
	}
	candidate := l.ranges[i-1]
	if addr > candidate.end {
		return 0, "", false
	}
	return candidate.asn, candidate.org, true
}

// ipToU32 converts a dotted-quad IPv4 address string to a uint32.
// Returns an error for any other form (IPv6, hostnames, etc.).
func ipToU32(s string) (uint32, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return 0, fmt.Errorf("asn: invalid IP %q", s)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, fmt.Errorf("asn: not an IPv4 address %q", s)
	}
	return binary.BigEndian.Uint32(ip4), nil
}
