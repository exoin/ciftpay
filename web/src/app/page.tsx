import { redirect } from "next/navigation";

/** The app opens straight on Today (design-system §1 rule 3). The shell redirects to /login if signed out. */
export default function Index() {
  redirect("/today");
}
