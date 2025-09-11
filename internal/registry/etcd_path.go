package registry

import (
	"fmt"
	"strings"

	"github.com/auto-dns/docker-coredns-sync/internal/util"
)

func keyBaseForFQDN(prefix, fqdn string) string {
	prefix = strings.TrimRight(prefix, "/")
	trimmed := strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	parts := strings.Split(trimmed, ".")
	parts = util.Reverse(parts)
	return fmt.Sprintf("%s/%s", prefix, strings.Join(parts, "/"))
}

// From a full etcd key to FQDN (handles trailing xNN segment)
func fqdnFromKey(prefix, key string) string {
	prefix = strings.TrimRight(prefix, "/")
	path := strings.TrimPrefix(key, prefix)
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if n := len(parts); n > 0 && strings.HasPrefix(parts[n-1], "x") {
		parts = parts[:n-1]
	}
	parts = util.Reverse(parts)
	return strings.Join(parts, ".")
}
