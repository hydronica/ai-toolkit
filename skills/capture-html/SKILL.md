---
name: capture-html
description: >-
  Capture a whole web page as one PNG using the Cursor browser MCP
  (chrome-devtools). Falls back to viewport captures at scroll positions
  stitched into one image when lazy-loaded content (such as Grafana panels)
  comes out blank in a full-page capture. Use when the user asks for a
  screenshot, full-page image, or stitched capture of a web page, dashboard,
  or PR preview, or mentions capture_html or capture-html.
disable-model-invocation: true
---

# Capture HTML

**Load:** Invoke **`@capture-html`**. Capture only. Do not edit the page, change filters, save dashboards, or log in.

Produces one PNG of a full web page in `~/Downloads/`, suitable for attaching to a pull request or document.

## Policy (must follow)

- Use the **Cursor browser MCP** (`user-chrome-devtools`) only. Do not use Playwright, Puppeteer, or other headless browsers unless the user explicitly asks.
- **Stitch in the browser** via `evaluate_script` and Canvas (see step 7). Do not use Python, Pillow, ImageMagick, or other image libraries unless the user explicitly asks.
- Save output only to **`~/Downloads/`**. The MCP rejected `/tmp` and paths inside the repo; `~/Downloads` is the verified location.
- **Never enter credentials.** Do not try default or guessed passwords. If the page shows a login screen, stop and ask the user to sign in in the MCP browser tab.
- Keep the **viewport size constant** for the whole run. Resizing or reloading in a loop resets rendering.
- Do not loop on retries. Try one full-page capture, then switch to section capture.

## Procedure

1. **Select the page.** `list_pages`, then `select_page` with `bringToFront: true` on the target tab. Grafana and other lazy-rendering pages do not paint in a background tab. If the tab is on the wrong URL, `navigate_page` with `type: url`.
2. **Set the viewport.** `resize_page` to `1600` x `1000`. Record it; use the same size for every capture in the run.
3. **Wait for data.** `evaluate_script` waits about 10 seconds, then returns the count of `$` amounts or other expected text in `document.body.innerText`. Continue only when the count is above zero.
4. **Try one full-page capture.** `take_screenshot` with `fullPage: true`, `format: png`, and `filePath` set to an absolute `~/Downloads/<name>.png` path. Then `Read` the file.
   - Every section has content: done. Go to step 8.
   - Panels are blank: go to step 5.
5. **Measure the page.** `evaluate_script` returns:
   ```js
   () => ({
     pageH: Math.max(document.body.scrollHeight, document.documentElement.scrollHeight),
     viewportH: window.innerHeight,
     viewportW: window.innerWidth,
   })
   ```
6. **Capture sections.** Scroll positions are `0, step, 2*step, ...`, with `step = viewportH - 200` and the last position at `pageH - viewportH`. For each position:
   - `evaluate_script`: `window.scrollTo(0, y)`, then wait 3.5 seconds.
   - `take_screenshot` (viewport only, no `fullPage`) to `~/Downloads/<name>-part-<n>.png`.
   - Record `scrollY` with its file path.
   - `Read` the part. If it is blank, wait 6 more seconds and retake that part only.
7. **Stitch (browser Canvas).**
   1. **Encode sections.** For each part PNG, get base64 without line breaks:
      - macOS: `base64 -i "<path>" | tr -d '\n'`
      - Linux: `base64 -w0 "<path>"`
   2. **Register payload.** `evaluate_script` with `format: "script"`, `waitForStableDom: false`, assigning one JSON object to `window.__captureHtmlStitch`:
      ```js
      window.__captureHtmlStitch = {
        pageHeight: <pageH>,
        viewportWidth: 1600,
        viewportHeight: 1000,
        stickyTop: 170,
        sections: [
          { scrollY: 0, base64: "<part-1 base64>" },
          { scrollY: 800, base64: "<part-2 base64>" }
        ]
      };
      ```
      - `stickyTop: 170` skips the fixed header in every section after the first. Use `0` for pages without a fixed header.
      - Sections must be in strictly increasing `scrollY` order; the first must be `0`.
   3. **Composite.** `evaluate_script` with `format: "function"`, `waitForStableDom: false`, and `sourcePath` set to the bundled stitch script:
      - Installed: `"${HOME}/.cursor/skills/ai-toolkit/capture-html/scripts/stitch_sections.js"`
      - Repo clone: absolute path to `skills/capture-html/scripts/stitch_sections.js`
      Returns `{ width, height, scale, pngBase64 }`. On failure the script throws with `error: …` (gaps, out-of-order sections, bad dimensions). Fix the input and rerun; do not hand-edit the output.
   4. **Write PNG.** Decode `pngBase64` to the final path (macOS example):
      ```bash
      printf '%s' '<pngBase64>' | base64 -D -o "${HOME}/Downloads/<name>.png"
      ```
      On Linux use `base64 -d` instead of `base64 -D`.
8. **Verify and clean up.** `Read` the final PNG and check seams and content. Only after it looks right, delete the `-part-*.png` files you created.
9. **Report** the output path, pixel size (`width` x `height` from stitch), number of sections, `scale`, and any panels that stayed empty. If the image is 2x the viewport, that is expected on Retina.

## Failure modes

| Symptom | Fix |
|---------|-----|
| Full-page capture has blank panels | Expected for lazy-loaded dashboards. Use section capture (step 5). |
| Sections are still blank after 6 more seconds | The panel may have no data for this filter. Check the page text, then report it. |
| `take_screenshot` says the path is outside the workspace roots | Use `~/Downloads/`. |
| `Browser is already running for ...chrome-profile` | Another Chrome session holds the MCP profile. Ask the user to close it. |
| Tab shows a login page | Stop. Ask the user to sign in in the MCP tab. Never type credentials. |
| Panels paint only after the tab is in front | Run `select_page` with `bringToFront: true` again, then wait. |
| Stitch throws a gap or coverage error | Reduce the scroll step, or capture the missing position. |
| Stitch fails or times out on `evaluate_script` | Section PNGs may be too large for one MCP payload. Capture fewer, wider overlaps, or ask the user to allow a one-off ImageMagick stitch. |

## Example

A Grafana dashboard with a 2092px page height, captured in three sections at a 1600x1000 viewport, with a 170px fixed header:

- Scroll 0 saves `…-part-1.png`.
- Scroll 800 saves `…-part-2.png`.
- Scroll 1092 saves `…-part-3.png`.

Stitching produces a 3200x4184 PNG at 2x scale in `~/Downloads/`.