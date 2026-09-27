"use client";

import { useEffect } from "react";

/**
 * Initializes native mobile integration when running inside Capacitor Android / iOS.
 * - Sets status bar theme and background
 * - Handles Android hardware back button
 * - Sets viewport padding for notched screens
 */
export function NativeBridge() {
  useEffect(() => {
    // Only run if Capacitor bridge is present on the window
    if (typeof window === "undefined" || !(window as unknown as { Capacitor?: { isNativePlatform?: () => boolean } }).Capacitor) {
      return;
    }

    const { Capacitor } = window as unknown as {
      Capacitor: {
        isNativePlatform: () => boolean;
        getPlatform: () => string;
      };
    };

    if (!Capacitor.isNativePlatform()) return;

    // Dynamically import Capacitor native plugins to avoid bundling issues on web
    Promise.all([
      import("@capacitor/status-bar").catch(() => null),
      import("@capacitor/app").catch(() => null),
    ]).then(([statusBarModule, appModule]) => {
      if (statusBarModule) {
        const { StatusBar, Style } = statusBarModule;
        StatusBar.setStyle({ style: Style.Dark }).catch(() => {});
        StatusBar.setBackgroundColor({ color: "#0F172A" }).catch(() => {});
      }

      if (appModule) {
        const { App } = appModule;
        App.addListener("backButton", ({ canGoBack }) => {
          if (canGoBack) {
            window.history.back();
          } else {
            App.exitApp();
          }
        });
      }
    });
  }, []);

  return null;
}
