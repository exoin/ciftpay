import { getTranslations } from "next-intl/server";

export const dynamic = "force-static";

export default async function Offline() {
  const t = await getTranslations("common");
  return (
    <main className="mx-auto max-w-md px-4 py-16">
      <h1 className="font-display text-xl">CiftPay</h1>
      <p className="mt-2 text-ink-2">{t("offline")}</p>
    </main>
  );
}
