# cuse

A CLI tool to check your Cursor billing usage.

## What it shows

- Current billing period dates
- API (named model) usage percentage
- Auto-complete usage percentage
- Request usage (used/limit and percentage)
- Percentage of billing period elapsed (to help pace usage)
- On-demand spending (if enabled)

## Installation

### Pre-built binaries

Download the latest release for your platform from the [Releases](https://github.com/hydronica/AI-toolkit/releases) page.

### From source

```bash
make build cuse    # from repo root; builds to scripts/cuse
# or
go install cmd/cuse  # installs to $GOPATH/bin
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
Total usage:
  API (named):   42.3%
  Auto:          15.7%
  Requests:      56.2% (1124/2000)

On-demand:       $2,000.00 spent
```

## Options

- `-version` — Print version and exit
- `-debug` — Print raw API responses for troubleshooting
- `-browser firefox|chromium` — Browser for login. Defaults to Firefox on Linux and a Chromium-based browser (Chrome, Edge, Brave) on Windows/macOS.
