# docker-coredns-sync

Watches for Docker container events and syncs registered DNS entries to etcd for use with CoreDNS.

## Labels
- `coredns.enabled=true`
- `coredns.name=optional-key-name` (optional - falls back to container name)
- `coredns.domain.1=sub1.domain.com`
- `coredns.domain.2=sub2.domain.com`
- `coredns.overwrite=true` (optional, allows replacing a domain claimed by another host)

## Environment Variables
- `HOST_IP`: IP address to register in etcd
- `HOSTNAME`: Unique host name for tracking entries
- `ETCD_URL`: URL of the etcd server (e.g., http://192.168.0.10:2379)