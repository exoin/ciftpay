import type { Metadata } from "next";
import { Suspense } from "react";
import { getTranslations } from "next-intl/server";
import { LoginForm } from "./LoginForm";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("login");
  return { title: t("title") };
}

export default async function LoginPage() {
  const t = await getTranslations();
  return (
    <main className="mx-auto w-full max-w-md px-4 py-10 lg:mx-0 lg:ml-[var(--rail)] lg:py-16">
      <div className="font-display text-2xl font-semibold text-green">{t("app.name")}</div>
      <p className="mt-1 text-sm text-muted">{t("app.tagline")}</p>
      <div className="mt-8">
        <Suspense>
          <LoginForm />
        </Suspense>
      </div>
      <p className="mt-10 text-xs text-muted">{t("login.footer")}</p>
    </main>
  );
}
