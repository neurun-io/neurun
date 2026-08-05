import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft, ArrowRight, ThumbsDown, ThumbsUp } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DocsToc } from "@/components/docs/chrome";
import { Lead } from "@/components/docs/prose";
import { DOCS, DOCS_ORDER, docsNeighbours, docsTags } from "@/lib/docs/content";

export function generateStaticParams() {
  return DOCS_ORDER.map((slug) => ({ slug }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const page = DOCS[(await params).slug];
  if (!page) return {};
  return { title: `${page.title} — Neurun docs` };
}

export default async function DocsPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const page = DOCS[slug];
  if (!page) notFound();

  const { prev, next } = docsNeighbours(slug);

  return (
    <div className="flex gap-11 px-6 pt-9 pb-28 lg:px-9">
      <article className="min-w-0 flex-1 lg:max-w-[820px]">
        <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 font-mono text-micro text-fg-muted">
          <Link href="/docs" className="hover:text-fg">
            Neurun
          </Link>
          <span aria-hidden>/</span>
          <span>{page.group}</span>
          <span aria-hidden>/</span>
          <span className="text-fg-secondary">{page.label}</span>
        </nav>

        <header className="mt-4 flex flex-col gap-4">
          <h1 className="text-[44px] leading-[1.04] tracking-display">{page.title}</h1>
          <Lead>{page.lead}</Lead>
          <div className="flex flex-wrap items-center gap-1.5">{docsTags(page.tags)}</div>
        </header>

        <hr className="nr-measure my-8 border-t border-line" />

        <div className="nr-measure flex flex-col gap-5.5">{page.body}</div>

        <nav className="nr-measure mt-11 grid gap-3 sm:grid-cols-2">
          {prev ? (
            <Link
              href={`/docs/${prev.slug}`}
              className="group flex flex-col gap-1 rounded-lg border border-line p-3.5 transition-colors duration-120 ease-mech hover:border-line-default hover:bg-surface-panel"
            >
              <span className="nr-label flex items-center gap-1.5">
                <ArrowLeft aria-hidden className="size-3" strokeWidth={1.5} />
                Previous
              </span>
              <span className="text-sm text-fg">{prev.label}</span>
            </Link>
          ) : (
            <span />
          )}
          {next ? (
            <Link
              href={`/docs/${next.slug}`}
              className="group flex flex-col items-end gap-1 rounded-lg border border-line p-3.5 text-right transition-colors duration-120 ease-mech hover:border-line-default hover:bg-surface-panel sm:col-start-2"
            >
              <span className="nr-label flex items-center gap-1.5">
                Next
                <ArrowRight aria-hidden className="size-3" strokeWidth={1.5} />
              </span>
              <span className="text-sm text-fg">{next.label}</span>
            </Link>
          ) : null}
        </nav>

        <footer className="nr-measure mt-7 flex flex-wrap items-center gap-3 border-t border-line pt-4">
          <span className="font-mono text-micro text-fg-muted">{page.source}</span>
          <span className="hidden flex-1 sm:block" />
          {/* No feedback endpoint ships in this release, so these are disabled
              with a reason rather than swallowing a click that goes nowhere. */}
          <Button variant="ghost" size="sm" aria-disabled title="Page feedback is not collected yet">
            <ThumbsUp aria-hidden strokeWidth={1.5} />
            Useful
          </Button>
          <Button variant="ghost" size="sm" aria-disabled title="Page feedback is not collected yet">
            <ThumbsDown aria-hidden strokeWidth={1.5} />
            Not useful
          </Button>
          <p className="w-full text-micro text-fg-muted">
            Page feedback is not collected yet. Until it is, these are disabled rather than
            pretending to work — mail{" "}
            <a href="mailto:docs@neurun.dev" className="underline underline-offset-3">
              docs@neurun.dev
            </a>
            .
          </p>
        </footer>
      </article>

      <DocsToc items={page.toc} />
    </div>
  );
}
