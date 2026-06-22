package asn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const tsvFixture = "1.0.0.0\t1.0.0.255\t13335\tAU\tCLOUDFLARE\n10.0.0.0\t10.255.255.255\t64512\tZZ\tPrivateNet\n"

func writeTempTSV(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ip2asn-v4.tsv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempTSV: %v", err)
	}
	return path
}

func TestLoad_InRange(t *testing.T) {
	path := writeTempTSV(t, tsvFixture)
	lk, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, lk)

	asn, org, ok := lk.Lookup("1.0.0.128")
	require.True(t, ok, "expected ok=true for in-range IP 1.0.0.128")
	require.Equal(t, uint32(13335), asn)
	require.Equal(t, "CLOUDFLARE", org)
}

func TestLoad_RangeEnd(t *testing.T) {
	path := writeTempTSV(t, tsvFixture)
	lk, err := Load(path)
	require.NoError(t, err)

	// Exact end of range should still resolve
	asn, org, ok := lk.Lookup("1.0.0.255")
	require.True(t, ok)
	require.Equal(t, uint32(13335), asn)
	require.Equal(t, "CLOUDFLARE", org)
}

func TestLoad_OutOfRange(t *testing.T) {
	path := writeTempTSV(t, tsvFixture)
	lk, err := Load(path)
	require.NoError(t, err)

	_, _, ok := lk.Lookup("2.0.0.0")
	require.False(t, ok, "expected ok=false for out-of-range IP 2.0.0.0")
}

func TestLoad_SecondRange(t *testing.T) {
	path := writeTempTSV(t, tsvFixture)
	lk, err := Load(path)
	require.NoError(t, err)

	asn, org, ok := lk.Lookup("10.1.2.3")
	require.True(t, ok)
	require.Equal(t, uint32(64512), asn)
	require.Equal(t, "PrivateNet", org)
}

func TestLoad_MalformedLine(t *testing.T) {
	path := writeTempTSV(t, "not\tvalid\tline\n"+tsvFixture)
	lk, err := Load(path)
	require.NoError(t, err, "malformed lines should be skipped, not fatal")
	require.NotNil(t, lk)

	// The two valid ranges should still work
	_, _, ok := lk.Lookup("1.0.0.1")
	require.True(t, ok)
}

func TestLoad_EmptyFile(t *testing.T) {
	path := writeTempTSV(t, "")
	lk, err := Load(path)
	require.NoError(t, err)
	_, _, ok := lk.Lookup("1.0.0.1")
	require.False(t, ok)
}

func TestLookup_NilReceiver(t *testing.T) {
	var lk *Lookup
	asn, org, ok := lk.Lookup("1.0.0.1")
	require.False(t, ok)
	require.Equal(t, uint32(0), asn)
	require.Equal(t, "", org)
}

func TestIPToU32(t *testing.T) {
	cases := []struct {
		ip   string
		want uint32
		ok   bool
	}{
		{"0.0.0.0", 0, true},
		{"255.255.255.255", 0xffffffff, true},
		{"1.0.0.255", 0x010000ff, true},
		{"bad", 0, false},
		{"1.2.3", 0, false},
		{"1.2.3.4.5", 0, false},
		{"256.0.0.1", 0, false},
	}
	for _, c := range cases {
		v, err := ipToU32(c.ip)
		if c.ok {
			require.NoError(t, err, "ip=%s", c.ip)
			require.Equal(t, c.want, v, "ip=%s", c.ip)
		} else {
			require.Error(t, err, "ip=%s", c.ip)
		}
	}
}
