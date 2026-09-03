import { cookies, headers } from "next/headers";
import { getRequestConfig } from "next-intl/server";
import { defaultLocale, isLocale, LOCALE_COOKIE, type Locale } from "./config";

/**
 * Locale is a user preference (cookie), not a URL segment: merchants install
 * the PWA once and flip EN/SW in Settings. Falls back to Accept-Language.
 */
export default getRequestConfig(async () => {
  const jar = await cookies();
  const fromCookie = jar.get(LOCALE_COOKIE)?.value;
  let locale: Locale = defaultLocale;
  if (isLocale(fromCookie)) {
    locale = fromCookie;
  } else {
    const accept = (await headers()).get("accept-language") ?? "";
    if (/^sw\b|,\s*sw\b/i.test(accept)) locale = "sw";
  }
  return {
    locale,
    messages: (await import(`../../../messages/${locale}.json`)).default,
    timeZone: "Africa/Nairobi",
  };
});
