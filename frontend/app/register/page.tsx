"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { LoginScreen } from "@/components/auth/login-screen";
import { useSession } from "@/lib/session/store";

/** Where the site's sign-up CTAs land: the same surface, opened on sign-up. */
export default function RegisterPage() {
  const { status } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (status === "authenticated") router.replace("/overview");
  }, [status, router]);

  return <LoginScreen initialMode="register" />;
}
