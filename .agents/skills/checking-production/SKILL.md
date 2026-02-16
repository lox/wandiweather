---
name: checking-production
description: "Diagnoses production issues for WandiWeather on Fly.io. Use when investigating alerts, outages, health check failures, or production data problems."
---

# Checking Production

Diagnose production issues for WandiWeather running on Fly.io.

## Status Page

Uptime Kuma status page: https://status.wandiweather.com

Monitors:
- `wandiweather.com` — site reachability
- `wandiweather.com/health` — health endpoint (returns JSON with station staleness)

## Health Endpoint

`GET https://wandiweather.com/health` returns JSON:

```json
{
  "status": "ok|degraded|error",
  "stations": [
    {"station_id": "IWANDI23", "last_seen": "...", "age_minutes": 4, "stale": false}
  ]
}
```

- **200 OK** — all stations reporting within 60 minutes
- **503 Service Unavailable** — at least one station stale (>60 min) or DB error
- Status monitor alerts fire on non-200 responses

## Investigation Workflow

### 1. Check current health
```bash
curl -s https://wandiweather.com/health | python3 -m json.tool
```

### 2. Check status page
Read https://status.wandiweather.com for uptime percentages and recent incidents.

### 3. Check Fly logs for errors
```bash
# IMPORTANT: Always use --no-tail to avoid streaming/hanging
fly logs --app wandiweather --no-tail 2>&1 | tail -200

# Filter for errors
fly logs --app wandiweather --no-tail 2>&1 | grep -iE 'error|fail|panic|stale' | tail -30

# Check for restarts
fly logs --app wandiweather --no-tail 2>&1 | grep -iE 'starting|restart|machine' | tail -20
```

### 4. Check machine status
```bash
fly machine list --app wandiweather
fly machine status <MACHINE_ID> --app wandiweather
```

### 5. Check observation gaps in the database
The prod container does **not** have `sqlite3`. Pull the database locally first:
```bash
task pull-db
```

Then query for observation gaps:
```sql
-- Gaps > 30 minutes in last 14 days
WITH gaps AS (
  SELECT station_id,
         observed_at,
         LAG(observed_at) OVER (PARTITION BY station_id ORDER BY observed_at) as prev_obs,
         CAST((julianday(observed_at) - julianday(LAG(observed_at) OVER (PARTITION BY station_id ORDER BY observed_at))) * 24 * 60 AS INTEGER) as gap_min
  FROM observations
  WHERE observed_at > datetime('now', '-14 days')
)
SELECT station_id, datetime(prev_obs) as gap_start, datetime(observed_at) as gap_end, gap_min
FROM gaps
WHERE gap_min > 30
ORDER BY gap_min DESC
LIMIT 30;
```

### 6. Check ingest health
```sql
-- Ingest success rate last 24h
SELECT source, endpoint, COUNT(*) as total, SUM(success) as ok,
       printf('%.1f%%', 100.0 * SUM(success) / COUNT(*)) as rate
FROM ingest_runs
WHERE started_at > datetime('now', '-1 day')
GROUP BY source, endpoint;
```

## Common Causes

| Symptom | Likely Cause |
|---------|-------------|
| /health returning 503 intermittently | Transient network timeouts between status monitor and Fly edge — not a real outage |
| Single station stale | PWS WiFi dropout or Weather Underground API issue — usually self-resolves |
| All stations stale | App crash, machine stopped, or WU API key issue |
| Machine restarted | Check `fly machine status` event logs for OOM or deploy |
| No recent observations | Check `fly logs` for scheduler errors |

## Key Facts

- App: `wandiweather` on Fly.io, region `syd`
- Auto-stop is **off**, min machines = 1
- Observations poll every 5 minutes
- 4 stations: IWANDI23, IBRIGH180, IWANDI25, IHARRI19
- Stale threshold: 60 minutes (any station triggers degraded)
