"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { ProfileForm, type ProfileValues } from "@/components/browser-profiles/profile-form";
import { InlineError } from "@/components/neurun/error-panel";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { useCreateBrowserProfileMutation } from "@/lib/api/queries";

export default function NewBrowserProfilePage() {
  const router = useRouter();
  const create = useCreateBrowserProfileMutation();

  function submit(values: ProfileValues) {
    create.mutate(values, {
      onSuccess: (profile) => {
        toast.success("Browser profile created");
        router.push(`/browser-profiles/${profile.id}`);
      },
    });
  }

  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Browser profiles", href: "/browser-profiles" }]}
        title="New profile"
      />
      <div className="p-6">
        <Panel label="Profile">
          <ProfileForm
            submitLabel="Create profile"
            pending={create.isPending}
            error={create.isError ? <InlineError error={create.error} /> : null}
            onSubmit={submit}
            onCancel={() => router.push("/browser-profiles")}
          />
        </Panel>
      </div>
    </div>
  );
}
