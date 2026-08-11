"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { LoginScreen } from "@/components/auth/login-screen";
import { useSession } from "@/lib/session/store";

export default function AuthPage() {
  const { session, isLoading } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (session) router.replace("/overview");
  }, [session, isLoading, router]);

  return <LoginScreen />;
}
