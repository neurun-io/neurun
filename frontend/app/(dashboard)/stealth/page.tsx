import { RoadmapRoute } from "@/components/neurun/feedback";
import { ROADMAP } from "@/lib/roadmap";

export default function Page() {
  return <RoadmapRoute {...ROADMAP.stealth} />;
}
