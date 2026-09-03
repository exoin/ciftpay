import { getTranslations } from "next-intl/server";

export default async function ReceiptNotFound() {
  const t = await getTranslations("receipt");
  return (
    <main className="mx-auto w-full max-w-[var(--receipt-w)] px-4 py-16">
      <div className="perforated-top perforated-bottom bg-paper-2 px-5 py-8">
        <h1 className="font-display text-xl">{t("notFound")}</h1>
        <p className="mt-2 text-sm text-ink-2">{t("notFoundLead")}</p>
      </div>
    </main>
  );
}
