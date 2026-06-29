package labeler

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewCIDRNamer(t *testing.T) {
	t.Parallel()

	t.Run("accepts valid mappings", func(t *testing.T) {
		t.Parallel()
		namer, err := NewCIDRNamer([]CIDRMapping{
			{CIDR: "172.20.5.0/24", Name: "postgres"},
			{CIDR: "10.0.0.0/8", Name: "internal"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, namer)
	})

	t.Run("rejects an invalid CIDR", func(t *testing.T) {
		t.Parallel()
		_, err := NewCIDRNamer([]CIDRMapping{{CIDR: "not-a-cidr", Name: "x"}})
		assert.Error(t, err)
	})

	t.Run("nil mappings yield an empty namer", func(t *testing.T) {
		t.Parallel()
		namer, err := NewCIDRNamer(nil)
		assert.NoError(t, err)
		assert.Equal(t, "", namer.Lookup(netip.MustParseAddr("1.2.3.4")))
	})
}

func TestCIDRNamerLookup(t *testing.T) {
	t.Parallel()

	namer, err := NewCIDRNamer([]CIDRMapping{
		{CIDR: "172.20.0.0/16", Name: "broad"},
		{CIDR: "172.20.5.0/24", Name: "postgres"},
	})
	assert.NoError(t, err)

	t.Run("returns the name of a matching prefix", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "postgres", namer.Lookup(netip.MustParseAddr("172.20.5.10")))
	})

	t.Run("prefers the longest (most specific) prefix", func(t *testing.T) {
		t.Parallel()
		// 172.20.5.10 is in both /16 and /24; the /24 must win regardless of
		// the order the mappings were supplied.
		assert.Equal(t, "postgres", namer.Lookup(netip.MustParseAddr("172.20.5.10")))
		assert.Equal(t, "broad", namer.Lookup(netip.MustParseAddr("172.20.9.1")))
	})

	t.Run("returns empty string when nothing matches", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", namer.Lookup(netip.MustParseAddr("8.8.8.8")))
	})
}

func podWithLabels(labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels}}
}

func TestResolveName(t *testing.T) {
	t.Parallel()

	namer, err := NewCIDRNamer([]CIDRMapping{{CIDR: "172.20.5.0/24", Name: "postgres"}})
	assert.NoError(t, err)
	labeler := NewLabeler(nil, nil, false, namer)

	podIP := netip.MustParseAddr("172.20.5.10")
	otherIP := netip.MustParseAddr("8.8.8.8")

	t.Run("name and component combine into name-component", func(t *testing.T) {
		t.Parallel()
		pod := podWithLabels(map[string]string{
			"app.kubernetes.io/name":      "redis",
			"app.kubernetes.io/component": "primary",
		})
		// The app column is ignored when the name-component form resolves.
		assert.Equal(t, "redis-primary", labeler.resolveName(pod, "redis", podIP))
	})

	t.Run("app column is used when component is missing", func(t *testing.T) {
		t.Parallel()
		pod := podWithLabels(map[string]string{"app.kubernetes.io/name": "redis"})
		assert.Equal(t, "redis", labeler.resolveName(pod, "redis", podIP))
	})

	t.Run("falls back to CIDR name when there is no pod and no app", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "postgres", labeler.resolveName(nil, "", podIP))
	})

	t.Run("app column takes precedence over a CIDR match", func(t *testing.T) {
		t.Parallel()
		// podIP matches the postgres CIDR, but a non-empty app wins.
		assert.Equal(t, "redis", labeler.resolveName(nil, "redis", podIP))
	})

	t.Run("falls back to extra fallbacks in order", func(t *testing.T) {
		t.Parallel()
		// No pod, no app, no CIDR match: first non-empty extra fallback wins.
		assert.Equal(t, "s3", labeler.resolveName(nil, "", otherIP, "s3", "ec2"))
		assert.Equal(t, "ec2", labeler.resolveName(nil, "", otherIP, "", "ec2"))
	})

	t.Run("falls back to unknown when nothing matches", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "UNKNOWN", labeler.resolveName(nil, "", otherIP))
	})

	t.Run("works without a CIDR namer configured", func(t *testing.T) {
		t.Parallel()
		noNamer := NewLabeler(nil, nil, false)
		assert.Equal(t, "UNKNOWN", noNamer.resolveName(nil, "", podIP))
	})
}
