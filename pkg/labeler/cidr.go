package labeler

import (
	"fmt"
	"net/netip"
	"sort"
)

// CIDRMapping maps an IP prefix to a human-readable name. It is the
// config-facing representation of a single cidr_names entry.
type CIDRMapping struct {
	CIDR string `yaml:"cidr"`
	Name string `yaml:"name"`
}

// cidrEntry is a parsed CIDRMapping.
type cidrEntry struct {
	prefix netip.Prefix
	name   string
}

// CIDRNamer resolves an IP to a configured name based on CIDR membership.
type CIDRNamer struct {
	entries []cidrEntry
}

// NewCIDRNamer parses the given mappings into a CIDRNamer. Entries are sorted
// by prefix length descending so that Lookup returns the longest (most
// specific) match. A nil or empty mappings slice yields an empty namer.
func NewCIDRNamer(mappings []CIDRMapping) (*CIDRNamer, error) {
	entries := make([]cidrEntry, 0, len(mappings))
	for _, m := range mappings {
		prefix, err := netip.ParsePrefix(m.CIDR)
		if err != nil {
			return nil, fmt.Errorf("invalid cidr_names entry %q: %w", m.CIDR, err)
		}
		entries = append(entries, cidrEntry{prefix: prefix.Masked(), name: m.Name})
	}

	// Sort by prefix length descending so the longest prefix match wins.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].prefix.Bits() > entries[j].prefix.Bits()
	})

	return &CIDRNamer{entries: entries}, nil
}

// Lookup returns the name of the most specific configured prefix containing ip,
// or an empty string if none match.
func (c *CIDRNamer) Lookup(ip netip.Addr) string {
	for _, e := range c.entries {
		if e.prefix.Contains(ip) {
			return e.name
		}
	}

	return ""
}
