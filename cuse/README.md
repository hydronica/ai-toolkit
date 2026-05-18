# cuse

A CLI tool to check your Cursor billing usage.

## What it shows

- Current billing period dates
- API (named model) usage percentage
- Auto-complete usage percentage
- Percentage of billing period elapsed (to help pace usage)
- On-demand spending (if enabled)

## Installation

```bash
go install .
```

## Usage

```bash
cuse
```

On first run (or when your session expires), cuse will open a browser window for you to log in to cursor.com. After authenticating, the session cookie is saved to `.env` so subsequent runs work automatically.

Example output:

```
Cursor usage
-------------
Billing period:  2026-05-01 → 2026-06-01
Total usage:     (period elapsed: 58.1% — stay under this)
  API (named):   42.3%
  Auto:          15.7%

On-demand:       $0.00 spent
```

## Options

- `-debug` — Print raw API responses for troubleshooting
