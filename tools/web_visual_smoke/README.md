# Web Visual Smoke

Lightweight Chrome DevTools screenshot harness for GoERP `/web`.

It verifies:

- desktop app launcher
- desktop Settings
- desktop Technical > Server Actions list
- desktop Technical > Server Actions form
- desktop search dropdown
- mobile app launcher
- mobile Technical > Server Actions list cards

It does not use Odoo Enterprise or OI source/assets. It captures only the running GoERP UI.

## Run

Start GoERP:

```sh
GORP_HTTP_ADDR=127.0.0.1:8073 go run ./cmd/gorpd serve
```

Run the harness:

```sh
node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/web_visual_smoke
```

Outputs:

- `reports/web_visual_smoke/*.png`
- `reports/web_visual_smoke/manifest.json`

The manifest contains selector counts, screenshot hashes, viewport sizes, and redacted URL metadata. It must not contain source contents or secrets.

## Baseline Compare

Create or refresh a baseline:

```sh
node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/web_visual_smoke --baseline-dir=reports/web_visual_baseline --update-baseline
```

Compare against a baseline:

```sh
node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/web_visual_smoke --baseline-dir=reports/web_visual_baseline
```

Baseline comparison uses exact screenshot SHA-256 hashes. Use it only on a pinned browser/runtime in CI.

## CI Use

Recommended CI sequence:

```sh
make ci
GORP_HTTP_ADDR=127.0.0.1:8073 go run ./cmd/gorpd serve > /tmp/gorpd-visual.log 2>&1 &
server_pid=$!
trap 'kill $server_pid' EXIT
until curl -fsS http://127.0.0.1:8073/web/health >/dev/null; do sleep 1; done
node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/web_visual_smoke
```

For regression gating, add `--baseline-dir` after a stable baseline is committed or provisioned as a CI artifact.

## Focused Test

```sh
node --test tools/web_visual_smoke/run.test.mjs
```
