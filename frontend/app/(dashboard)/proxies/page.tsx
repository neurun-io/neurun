import { UnbuiltRoute } from "@/components/neurun/feedback";
import { ROADMAP } from "@/lib/roadmap";

export default function Page() {
  return <UnbuiltRoute {...ROADMAP.proxies} />;
}
