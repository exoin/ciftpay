import { FlatCompat } from "@eslint/eslintrc";

const compat = new FlatCompat({ baseDirectory: import.meta.dirname });

const config = [
  {
    ignores: [".next/**", "node_modules/**", "public/sw.js", "src/lib/api/schema.d.ts", "playwright-report/**", "test-results/**", "next-env.d.ts"],
  },
  ...compat.extends("next/core-web-vitals", "next/typescript"),
  {
    files: ["src/**/*.{ts,tsx}"],
    rules: {
      "@typescript-eslint/consistent-type-imports": "error",
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
      // N5 in plan.md: no hard-coded user-facing strings in app screens.
      // Warning level until the string inventory settles; punctuation and the
      // currency label are allowed as literals.
      "react/jsx-no-literals": [
        "warn",
        { noStrings: true, ignoreProps: true, allowedStrings: ["·", "···", "—", "−", "•", "/", ":", ".", "(", ")", "%", "…", "404", "KES", "+254", "CiftPay"] },
      ],
    },
  },
  {
    // Receipts, and the Safaricom authorization letter (an English-only legal document), are not translated.
    files: ["src/components/receipt/**/*.tsx", "src/app/r/**/*.{ts,tsx}", "src/app/(auth)/onboarding/letter/**/*.tsx", "src/**/*.test.{ts,tsx}"],
    rules: { "react/jsx-no-literals": "off" },
  },
];

export default config;
