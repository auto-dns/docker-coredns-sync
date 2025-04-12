# Test cases

## Case 1

Goal: Basic A record registration.

Expected:
* One A record in etcd: app1.example.com -> 192.168.1.100
* Owned by this host

```bash
docker run -d --rm --name test-a1 --label coredns.enabled=true --label coredns.A.name=app1.example.com --label coredns.A.value=192.168.1.100 traefik/whoami
```

Verify:
* Logs: look for [reconciler] Adding new record
```bash
docker exec -it etcd etcdctl get --prefix /skydns
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
docker run -d --rm --name test-a2 --label coredns.enabled=true --label coredns.A.name=app1.example.com --label coredns.A.value=192.168.1.100 traefik/whoami
```

## Case 4

Expected:
* Should be accepted and registered
* Now 2 A records under app1.example.com (for 192.168.1.100 and .101)

```bash
docker run -d --rm --name test-a3 --label coredns.enabled=true --label coredns.A.name=app1.example.com --label coredns.A.value=192.168.1.101 traefik/whoami
```

## Case 5

Expected:
* ❌ Rejected due to conflict with existing A record
* Logs show validation error: cannot add a CNAME record when an A record exists with the same name

```bash
docker run -d --rm --name test-c1 --label coredns.enabled=true --label coredns.CNAME.name=app1.example.com --label coredns.CNAME.value=another.example.com traefik/whoami
```

## Case 6

Expected:
* ✅ Added successfully
* app2.example.com resolves to same as app1.example.com

```bash
docker run -d --rm --name test-c2 --label coredns.enabled=true --label coredns.CNAME.name=app2.example.com --label coredns.CNAME.value=app1.example.com traefik/whoami
```

## Case 7

Expected:
* Replaces previous A records under app1.example.com
* Old records are removed
* Logs show: [reconciler] Forcibly overriding record owned by...

```bash
docker run -d --rm --name test-a4 --label coredns.enabled=true --label coredns.A.name=app1.example.com --label coredns.A.value=192.168.1.102 --label coredns.A.force=true traefik/whoami
```

## Case 8

Expected:
* Record app1.example.com -> 192.168.1.101 is removed
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

# Additional Corner Case Test Cases

## Case 10

Goal: CNAME record should override all A records with same name if it is older and has `force`.

Expected:
* All A records for `app3.example.com` are evicted.
* CNAME record `app3.example.com -> target.example.com` is registered.
* Logs show evictions and addition of the CNAME.

```bash
docker run -d --rm --name test-a5 --label coredns.enabled=true --label coredns.A.name=app3.example.com --label coredns.A.value=192.168.1.111 traefik/whoami
docker run -d --rm --name test-a6 --label coredns.enabled=true --label coredns.A.name=app3.example.com --label coredns.A.value=192.168.1.112 traefik/whoami
docker run -d --rm --name test-c3 --label coredns.enabled=true --label coredns.CNAME.name=app3.example.com --label coredns.CNAME.value=target.example.com --label coredns.CNAME.force=true traefik/whoami
```

## Case 11

Goal: A record should *not* override CNAME if it is newer and lacks `force`.

Expected:
* Existing CNAME record remains.
* A record attempt is skipped.
* Logs show [reconciler] Record conflict (not overriding)...

```bash
docker run -d --rm --name test-c4 --label coredns.enabled=true --label coredns.CNAME.name=app4.example.com --label coredns.CNAME.value=target.example.com traefik/whoami
docker run -d --rm --name test-a7 --label coredns.enabled=true --label coredns.A.name=app4.example.com --label coredns.A.value=192.168.1.113 traefik/whoami
```

## Case 12

Goal: A record with force and newer timestamp should override older A record from another container.

Expected:
* Older A record is evicted.
* New A record is added.
* Logs show eviction and new addition.

```bash
docker run -d --rm --name test-a8 --label coredns.enabled=true --label coredns.A.name=app5.example.com --label coredns.A.value=192.168.1.114 traefik/whoami
docker run -d --rm --name test-a9 --label coredns.enabled=true --label coredns.A.name=app5.example.com --label coredns.A.value=192.168.1.114 --label coredns.A.force=true traefik/whoami
```

## Case 13

Goal: CNAME record with older timestamp but without force should not override A records with force.

Expected:
* A records remain.
* CNAME attempt is skipped.
* Logs show [reconciler] Record conflict (not overriding)...

```bash
docker run -d --rm --name test-a10 --label coredns.enabled=true --label coredns.A.name=app6.example.com --label coredns.A.value=192.168.1.115 --label coredns.A.force=true traefik/whoami
docker run -d --rm --name test-c5 --label coredns.enabled=true --label coredns.CNAME.name=app6.example.com --label coredns.CNAME.value=target.example.com traefik/whoami
```

## Case 14

Goal: Identical record registered from different container with no force â newer one should not override.

Expected:
* Original remains.
* Duplicate is skipped.
* Logs show [reconciler] Record conflict (not overriding)...

```bash
docker run -d --rm --name test-a11 --label coredns.enabled=true --label coredns.A.name=app7.example.com --label coredns.A.value=192.168.1.116 traefik/whoami
docker run -d --rm --name test-a12 --label coredns.enabled=true --label coredns.A.name=app7.example.com --label coredns.A.value=192.168.1.116 traefik/whoami
```

## Cleanup

```bash
docker rm -f test-a5 test-a6 test-c3 test-c4 test-a7 test-a8 test-a9 test-a10 test-c5 test-a11 test-a12
```
