import { getLocale, getTranslations } from "next-intl/server";
import { receiptCss } from "@/components/receipt/css";
import { notFoundDocumentHtml, receiptDocumentHtml, type PublicReceipt } from "@/components/receipt/html";
import { qrSvg } from "@/components/receipt/qr";
import { serverApi } from "@/lib/api/client";

/**
 * Public buyer receipt. A route handler rather than a page: the document is
 * plain HTML with inlined CSS and no JavaScript, so it stays under 30 KB and
 * renders on a feature phone browser (design-system §11, N6). The only centred
 * layout in the product (it is a document, not an app screen).
 */
export const dynamic = "force-dynamic";

type Ctx = { params: Promise<{ code: string }> };

const CODE_RE = /^[A-Z0-9]{6,12}$/i;

const HTML_HEADERS = {
  "Content-Type": "text/html; charset=utf-8",
  "X-Robots-Tag": "noindex, nofollow",
} as const;

async function load(code: string): Promise<PublicReceipt | null> {
  if (!CODE_RE.test(code)) return null;
  const { data, response } = await serverApi().GET("/r/{code}", { params: { path: { code: code.toUpperCase() } } });
  if (response.status === 404 || !data) return null;
  return data;
}

export async function GET(_req: Request, { params }: Ctx): Promise<Response> {
  const { code } = await params;
  const [rc, t, locale, css] = await Promise.all([load(code), getTranslations("receipt"), getLocale(), receiptCss()]);

  if (!rc) {
    return new Response(notFoundDocumentHtml({ t, locale, css }), {
      status: 404,
      headers: { ...HTML_HEADERS, "Cache-Control": "no-store" },
    });
  }

  const qr = rc.kra_qr_payload ? await qrSvg(rc.kra_qr_payload) : null;
  const html = receiptDocumentHtml({ rc, qrSvg: qr, t, locale, css });
  return new Response(html, {
    status: 200,
    headers: {
      ...HTML_HEADERS,
      // State flips pending → verified within minutes; keep caches short.
      "Cache-Control": "private, max-age=60",
      Vary: "Cookie, Accept-Language",
    },
  });
}

export async function POST(req: Request, { params }: Ctx): Promise<Response> {
  const { code } = await params;
  let buyerPin = "";
  let buyerName = "";
  const contentType = req.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    const body = await req.json();
    buyerPin = body.buyer_pin || "";
    buyerName = body.buyer_name || "";
  } else {
    const formData = await req.formData();
    buyerPin = String(formData.get("buyer_pin") || "");
    buyerName = String(formData.get("buyer_name") || "");
  }

  buyerPin = buyerPin.trim().toUpperCase();
  buyerName = buyerName.trim();

  if (!buyerPin) {
    return Response.redirect(new URL(`/r/${code}?error=missing_pin`, req.url), 303);
  }

  const { data, response } = await serverApi().POST("/r/{code}/claim", {
    params: { path: { code: code.toUpperCase() } },
    body: { buyer_pin: buyerPin, buyer_name: buyerName || undefined },
  });

  if (!response.ok || !data) {
    return Response.redirect(new URL(`/r/${code}?error=invalid_pin`, req.url), 303);
  }

  return Response.redirect(new URL(`/r/${code}?claimed=1`, req.url), 303);
}
