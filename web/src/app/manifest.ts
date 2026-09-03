import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/today",
    name: "CiftPay",
    short_name: "CiftPay",
    description: "Every M-Pesa payment becomes a KRA eTIMS invoice.",
    start_url: "/today",
    scope: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#F6F1E7",
    theme_color: "#F6F1E7",
    lang: "en",
    dir: "ltr",
    categories: ["business", "finance"],
    icons: [
      { src: "/icons/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
      { src: "/icons/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icons/icon-512.png", sizes: "512x512", type: "image/png" },
      { src: "/icons/maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
    shortcuts: [
      { name: "Record a sale", url: "/today?action=sale" },
      { name: "Needs attention", url: "/attention" },
    ],
  };
}
