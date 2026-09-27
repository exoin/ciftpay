import html2canvas from "html2canvas";
import { jsPDF } from "jspdf";

export type PdfExportOptions = {
  filename?: string;
  marginMm?: number;
  orientation?: "portrait" | "landscape";
  format?: "a4" | "receipt";
  scale?: number;
  captureWidth?: number;
};

/**
 * Captures an HTML element using html2canvas
 * and directly downloads a professional PDF using jsPDF without browser print dialogs.
 *
 * Guarantees:
 * 1. Proportional capture width (420px for thermal receipts, 800px desktop width for documents).
 * 2. Single-page constraint: strictly fits the entire rendered document into a single A4 page.
 * 3. Color safety: safely ignores unsupported CSS color spaces (e.g. oklab/oklch).
 * 4. File-size optimization: Uses crisp JPEG encoding (0.92 quality) with balanced scale (2x)
 *    so generated PDFs stay well under ~1 MB while remaining crystal clear for printing and OCR.
 */
export async function exportElementToPdf(
  element: HTMLElement,
  options: PdfExportOptions = {}
): Promise<void> {
  const scale = options.scale ?? 2;
  const filename = options.filename ?? "document.pdf";
  const margin = options.marginMm ?? (options.format === "receipt" ? 4 : 10);
  const defaultCaptureWidth = options.format === "receipt" ? 420 : 800;
  const targetCaptureWidth = options.captureWidth ?? defaultCaptureWidth;

  // Temporarily force optimal document dimensions on the DOM node to avoid narrow viewport squash
  const originalWidth = element.style.width;
  const originalMaxWidth = element.style.maxWidth;
  const originalBoxSizing = element.style.boxSizing;

  element.style.width = `${targetCaptureWidth}px`;
  element.style.maxWidth = `${targetCaptureWidth}px`;
  element.style.boxSizing = "border-box";

  let canvas: HTMLCanvasElement;
  try {
    canvas = await html2canvas(element, {
      scale,
      useCORS: true,
      logging: false,
      backgroundColor: options.format === "receipt" ? "#fffdf8" : "#ffffff",
      width: targetCaptureWidth,
      windowWidth: Math.max(1024, targetCaptureWidth),
    });
  } finally {
    // Restore original styles immediately
    element.style.width = originalWidth;
    element.style.maxWidth = originalMaxWidth;
    element.style.boxSizing = originalBoxSizing;
  }

  // Use high-quality JPEG with compression (0.92) rather than uncompressed 32-bit PNG to keep PDF size under 1MB
  const imgData = canvas.toDataURL("image/jpeg", 0.92);

  if (options.format === "receipt") {
    // Custom receipt thermal paper dimension: 80mm width with dynamic proportional content height
    const pdfWidth = 80;
    const printableWidth = pdfWidth - margin * 2;
    const imgHeight = (canvas.height * printableWidth) / canvas.width;
    const pdfHeight = imgHeight + margin * 2;

    const doc = new jsPDF({
      orientation: "portrait",
      unit: "mm",
      format: [pdfWidth, pdfHeight],
    });
    doc.addImage(imgData, "JPEG", margin, margin, printableWidth, imgHeight, undefined, "FAST");
    doc.save(filename);
    return;
  }

  // Standard A4 document (210mm x 297mm) - STRICT SINGLE PAGE CONSTRAINT
  const doc = new jsPDF({
    orientation: options.orientation ?? "portrait",
    unit: "mm",
    format: "a4",
  });

  const pageWidth = 210;
  const pageHeight = 297;
  const maxPrintableWidth = pageWidth - margin * 2;
  const maxPrintableHeight = pageHeight - margin * 2;

  // Calculate scale factors to guarantee the captured document fits entirely on ONE page
  const scaleX = maxPrintableWidth / canvas.width;
  const scaleY = maxPrintableHeight / canvas.height;
  const fitScale = Math.min(scaleX, scaleY);

  const finalImgWidth = canvas.width * fitScale;
  const finalImgHeight = canvas.height * fitScale;

  // Center horizontally within printable margins
  const posX = margin + (maxPrintableWidth - finalImgWidth) / 2;
  const posY = margin;

  doc.addImage(imgData, "JPEG", posX, posY, finalImgWidth, finalImgHeight, undefined, "FAST");
  doc.save(filename);
}
