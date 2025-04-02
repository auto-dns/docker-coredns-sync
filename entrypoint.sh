#!/bin/bash
set -e

# Required ENV: HOST_IP, HOSTNAME, ETCD_URL

# Run main sync script
python /app/sync.py