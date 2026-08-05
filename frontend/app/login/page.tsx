"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { LoginScreen } from "@/components/auth/login-screen";
import { useSession } from "@/lib/session/store";

/**
 * The standalone sign-in surface the marketing site links to.
 *
 * The dashboard still gates in place, so a deep link survives signing in. This
 * route exists for people arriving from the public site with no destination in
 * mind, and sends them to the dashboard once the cookie is set.
 */
export default function LoginPage() {
  const { status } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (status === "authenticated") router.replace("/overview");
  }, [status, router]);

  return <LoginScreen />;
}
