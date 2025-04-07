# Test cases

## Case 1

Goal: Basic A record registration.

Expected:
* One A record in etcd: app1.***REMOVED*** -> 192.168.1.100
* Owned by this host

Verify:
* docker exec -it etcd etcdctl get --prefix ***REMOVED***/app1
* Logs: look for [reconciler] Adding new record

```bash
docker run -d --name test-a1 --label coredns.enabled=true --label coredns.A.name=app1.***REMOVED*** --label coredns.A.value=192.168.1.100 busybox sleep 9999
```

## Case 2

Expected:
* No change. It’s already registered by this container.
* Logs show [reconciler] Skipping record already owned by us

```bash
docker restart test-a1
```

## Case 3

Expected:
* Should not be added (duplicate name + IP)
* Logs show: [reconciler] Duplicate record with same name and value already exists... Skipping

```bash
docker run -d --name test-a2 --label coredns.enabled=true --label coredns.A.name=app1.***REMOVED*** --label coredns.A.value=192.168.1.100 busybox sleep 9999
```

## Case 4

Expected:
* Should be accepted and registered
* Now 2 A records under app1.***REMOVED*** (for 192.168.1.100 and .101)

Verify:
* docker exec -it etcd etcdctl get --prefix ***REMOVED***/app1
* Or query via CoreDNS: dig @<coredns_ip> app1.***REMOVED***

```bash
docker run -d --name test-a3 --label coredns.enabled=true --label coredns.A.name=app1.***REMOVED*** --label coredns.A.value=192.168.1.101 busybox sleep 9999
```

## Case 5

Expected:
* ❌ Rejected due to conflict with existing A record
* Logs show validation error: cannot add a CNAME record when an A record exists with the same name

```bash
docker run -d --name test-c1 --label coredns.enabled=true --label coredns.CNAME.name=app1.***REMOVED*** --label coredns.CNAME.value=another.***REMOVED*** busybox sleep 9999
```

## Case 6

Expected:
* ✅ Added successfully
* ***REMOVED*** resolves to same as app1.***REMOVED***

```bash
docker run -d --name test-c2 --label coredns.enabled=true --label coredns.CNAME.name=***REMOVED*** --label coredns.CNAME.value=app1.***REMOVED*** busybox sleep 9999
```

## Case 7

Expected:
* Replaces previous A records under app1.***REMOVED***
* Old records are removed
* Logs show: [reconciler] Forcibly overriding record owned by...

```bash
docker run -d --name test-a4 --label coredns.enabled=true --label coredns.A.name=app1.***REMOVED*** --label coredns.A.value=192.168.1.102 --label coredns.A.force=true busybox sleep 9999
```

## Case 8

Expected:
* Record app1.***REMOVED*** -> 192.168.1.101 is removed
* Logs show:
[sync_engine] Removing record due to stale state

```bash
docker stop test-a3
```

## Case 9

List all records

You should see only live, valid records. If something’s dangling, restart the sync app or wait for stale record cleanup.

```bash
docker exec -it etcd etcdctl get --prefix /skydns/
```

## Cleanup

```bash
docker rm -f test-a1 test-a2 test-a3 test-a4 test-c1 test-c2
```
