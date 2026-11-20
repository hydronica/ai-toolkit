/**
 * Stitch viewport screenshots (browser Canvas). Used via chrome-devtools
 * evaluate_script sourcePath after window.__captureHtmlStitch is set.
 *
 * Input shape (window.__captureHtmlStitch):
 *   pageHeight, viewportWidth, viewportHeight, stickyTop (CSS px),
 *   sections: [{ scrollY, base64 }] in increasing scrollY order; first at 0.
 *
 * Returns: { width, height, scale, pngBase64 } (no data: URL prefix).
 */
async () => {
  const input = window.__captureHtmlStitch;
  if (!input) {
    throw new Error("error: missing window.__captureHtmlStitch; run the payload step first");
  }

  const pageHeight = input.pageHeight;
  const viewportWidth = input.viewportWidth;
  const viewportHeight = input.viewportHeight;
  const stickyTop = input.stickyTop ?? 0;
  const sections = input.sections;

  if (!Array.isArray(sections) || sections.length === 0) {
    throw new Error("error: sections must be a non-empty array");
  }
  if (sections[0].scrollY !== 0) {
    throw new Error("error: the first section must be at scroll position 0");
  }
  for (let i = 1; i < sections.length; i++) {
    if (sections[i].scrollY <= sections[i - 1].scrollY) {
      throw new Error("error: sections must be in strictly increasing scroll order");
    }
  }

  function loadImage(base64) {
    return new Promise((resolve, reject) => {
      const img = new Image();
      img.onload = () => resolve(img);
      img.onerror = () => reject(new Error("error: failed to decode a section image"));
      img.src = `data:image/png;base64,${base64}`;
    });
  }

  const images = [];
  for (const { scrollY, base64 } of sections) {
    if (!base64) {
      throw new Error(`error: missing base64 for section at scroll ${scrollY}`);
    }
    images.push({ scrollY, img: await loadImage(base64) });
  }

  const scale = images[0].img.naturalWidth / viewportWidth;
  for (const { scrollY, img } of images) {
    if (Math.abs(img.naturalWidth / viewportWidth - scale) > 0.01) {
      throw new Error(
        `error: section at scroll ${scrollY} width ${img.naturalWidth}px does not match the other sections`
      );
    }
    if (Math.abs(img.naturalHeight - viewportHeight * scale) > 1) {
      throw new Error(
        `error: section at scroll ${scrollY} height ${img.naturalHeight}px does not match viewport height ${viewportHeight}`
      );
    }
  }

  const device = (css) => Math.round(css * scale);

  const starts = [0, ...images.slice(1).map(({ scrollY }) => scrollY + stickyTop)];
  const ends = [...starts.slice(1), pageHeight];

  const canvas = document.createElement("canvas");
  canvas.width = device(viewportWidth);
  canvas.height = device(pageHeight);
  const ctx = canvas.getContext("2d");
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  for (let index = 0; index < images.length; index++) {
    const { scrollY, img } = images[index];
    const start = starts[index];
    const end = ends[index];
    if (end <= start) {
      continue;
    }
    const top = device(start) - device(scrollY);
    const bottom = device(end) - device(scrollY);
    if (top < 0) {
      throw new Error(`error: section ${index} starts above its scroll position`);
    }
    if (bottom > img.naturalHeight) {
      throw new Error(
        `error: section ${index} (scroll ${scrollY}) does not reach page y=${end}; reduce the scroll step or add a section`
      );
    }
    const sw = img.naturalWidth;
    const sh = bottom - top;
    ctx.drawImage(img, 0, top, sw, sh, 0, device(start), sw, sh);
  }

  if (ends[ends.length - 1] < pageHeight) {
    throw new Error("error: sections do not cover the whole page");
  }

  const pngBase64 = canvas.toDataURL("image/png").split(",")[1];
  return {
    width: canvas.width,
    height: canvas.height,
    scale,
    pngBase64,
  };
};