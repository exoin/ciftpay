import Link from "next/link";
import { getTranslations } from "next-intl/server";

export default async function NotFound() {
  const t = await getTranslations("common");
  return (
    <main className="mx-auto max-w-md px-4 py-16">
      <h1 className="font-display text-xl">404</h1>
      <p className="mt-2 text-ink-2">{t("noConnection")}</p>
      <Link href="/today" className="mt-6 inline-block underline">
        {t("back")}
      </Link>
    </main>
  );
}
