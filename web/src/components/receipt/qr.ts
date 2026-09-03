import QRCode from "qrcode";

/**
 * Server-side QR as inline SVG: 3 px module, ink on paper-2, no logo overlay
 * (design-system §10, KRA scanners). Returns markup safe to inline.
 */
export async function qrSvg(payload: string): Promise<string> {
  const svg = await QRCode.toString(payload, {
    type: "svg",
    errorCorrectionLevel: "M",
    margin: 0,
    color: { dark: "#14130f", light: "#fffdf800" },
  });
  // qrcode emits a fixed width/height; make it scale with its container.
  return svg.replace(/<svg([^>]*?)\swidth="[^"]*"\sheight="[^"]*"/, "<svg$1").replace("<svg", '<svg role="img" aria-hidden="true"');
}
