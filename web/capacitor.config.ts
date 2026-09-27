import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "com.ciftpay.app",
  appName: "CiftPay",
  webDir: "public",
  // In development and remote production, the app loads from the hosted origin or local Next.js server
  server: {
    androidScheme: "https",
    cleartext: true,
  },
  plugins: {
    StatusBar: {
      style: "DARK",
      backgroundColor: "#0F172A",
    },
    Keyboard: {
      resize: "body",
      resizeOnFullScreen: true,
    },
  },
};

export default config;
