"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { useCreateOrg } from "@/lib/auth";

const schema = z.object({
  name: z
    .string()
    .trim()
    .min(2, "Firm or practice name must be at least 2 characters.")
    .max(80, "Firm or practice name cannot exceed 80 characters."),
});

type Values = z.infer<typeof schema>;

export function AccountantForm({ onCreated }: { onCreated?: (org: Schemas["Org"]) => void }) {
  const tc = useTranslations("common");
  const locale = useLocale() as Schemas["Locale"];
  const router = useRouter();
  const create = useCreateOrg();

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: "" },
  });

  const apiError = create.error;
  let apiMessage: string | null = null;
  if (apiError instanceof ApiRequestError) {
    apiMessage = tc("errorGeneric", { message: apiError.message });
  } else if (apiError) {
    apiMessage = tc("noConnection");
  }

  return (
    <form
      noValidate
      className="space-y-5"
      onSubmit={form.handleSubmit(async (v) => {
        const org = await create.mutateAsync({
          name: v.name,
          role: "accountant",
          profile: "accountant",
          vat_registered: false,
          locale,
        });
        if (onCreated) {
          onCreated(org);
        } else {
          router.replace("/clients");
        }
      })}
    >
      <div>
        <h1 className="text-xl font-semibold text-ink">Tax Professional / Firm Profile</h1>
        <p className="mt-2 text-sm text-ink-2">
          Enter your name or practice firm name. You will be able to manage client workspaces, review ledgers, and download KRA iTax VAT return CSVs.
        </p>
      </div>

      <Field
        label="Accountant / Firm Name"
        hint="e.g. Mwangi & Associates CPA or Jane Kamau, Tax Consultant"
        error={form.formState.errors.name?.message}
        autoComplete="organization"
        maxLength={80}
        placeholder="Mwangi & Associates CPA"
        {...form.register("name")}
      />

      {apiMessage && (
        <div role="alert" className="rounded-md border border-[var(--color-danger)] bg-paper-2 p-3 text-xs text-[var(--color-danger)]">
          {apiMessage}
        </div>
      )}

      <div className="pt-2">
        <Button
          type="submit"
          variant="primary"
          block
          loading={create.isPending}
          disabled={create.isPending}
        >
          Create Practice & Go to Clients
        </Button>
      </div>
    </form>
  );
}
