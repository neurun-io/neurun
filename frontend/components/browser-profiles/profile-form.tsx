"use client";

import { useId, useState, type ReactNode, type SubmitEvent } from "react";
import { Check, ChevronsUpDown } from "lucide-react";

import { Callout } from "@/components/neurun/feedback";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useIdentityCatalogQuery } from "@/lib/api/queries";
import type {
  BrowserIdentity,
  BrowserProfile,
  CatalogGPU,
  IdentityCatalog,
} from "@/lib/api/resource-types";
import {
  browsersFor,
  browserVersionsFor,
  catalogDraft,
  deviceForModel,
  deviceNamed,
  devicesFor,
  fromDraft,
  gpusFor,
  isMobile,
  osVersionsFor,
  platformVersionsFor,
  toDraft,
  withBrowser,
  withDevice,
  withGeo,
  withOS,
  withOSVersion,
  withGPU,
  withRatio,
  withScreen,
  type IdentityDraft,
} from "@/lib/view/browser-identity";

export interface ProfileValues {
  name: string;
  /** Absent on create means the server draws one. */
  identity?: BrowserIdentity;
}

/**
 * Create and edit share one form, because the identity is the same fields
 * either way. A create screen that offers fewer settings than the edit screen is
 * why nobody's first profile is the one they wanted.
 */
export function ProfileForm({
  profile,
  submitLabel,
  pending,
  error,
  onSubmit,
  onCancel,
}: {
  /** The profile being edited. Absent creates a new one. */
  profile?: BrowserProfile;
  submitLabel: string;
  pending: boolean;
  error?: ReactNode;
  onSubmit: (values: ProfileValues) => void;
  onCancel?: () => void;
}) {
  const catalog = useIdentityCatalogQuery();
  const nameId = useId();
  const identityId = useId();

  const [name, setName] = useState(profile?.name ?? "");
  const [shown, setShown] = useState(false);
  const [draft, setDraft] = useState<IdentityDraft | null>(() =>
    profile?.identity ? toDraft(profile.identity) : null,
  );

  // A stored identity carries a model, not a handset. The catalogue arrives
  // after the first render, so the device is resolved here rather than in state:
  // the select opens on the phone that model shipped with.
  const bound =
    draft && catalog.data && !draft.device && draft.device_model
      ? { ...draft, device: deviceForModel(catalog.data, draft.device_model) ?? "" }
      : draft;

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    onSubmit({ name: name.trim(), identity: bound ? fromDraft(bound) : undefined });
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={nameId}>Name</Label>
          <Input
            id={nameId}
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
          />
        </div>

        <div className="space-y-1.5">
          <Label htmlFor={identityId}>Identity</Label>
          <div className="flex h-9 items-center gap-2.5 rounded-md border border-input px-3">
            <Switch
              id={identityId}
              disabled={!catalog.data}
              checked={shown}
              onCheckedChange={(on) => {
                setShown(on);
                if (!on || draft || !catalog.data) return;
                setDraft(
                  profile?.identity
                    ? toDraft(profile.identity, catalog.data)
                    : catalogDraft(catalog.data),
                );
              }}
            />
            <span className="text-sm text-fg-secondary">{shown ? "Shown" : "Hidden"}</span>
          </div>
          <Hint>
            {draft ? "What every session wears." : "Drawn from the catalogue if you leave it."}
          </Hint>
        </div>
      </div>

      {shown && bound && catalog.data ? (
        <IdentityFields
          draft={bound}
          catalog={catalog.data}
          setDraft={setDraft}
          notes={
            <>
              {bound.browser === "safari" ? (
                <Callout kind="note" title="Safari reports less than this">
                  Safari exposes no deviceMemory and no User-Agent Client Hints, so those
                  fields are carried but never read. Its WebGL pair is fixed to Apple.
                </Callout>
              ) : null}
              {profile?.identity?.proxy_set && !bound.proxy.trim() ? (
                <Callout kind="warning" title="The stored proxy will be cleared">
                  This profile has a proxy, and the API never returns it. Saving with the
                  field empty replaces it with none — retype it to keep one.
                </Callout>
              ) : null}
            </>
          }
        />
      ) : null}

      {draft && catalog.isPending ? <p className="text-fg-muted">Loading the catalogue…</p> : null}
      {catalog.isError ? (
        <Callout kind="warning" title="The identity catalogue did not load">
          Without it these fields cannot be bound to each other, so the identity is left
          alone rather than guessed at.
        </Callout>
      ) : null}

      {error}

      <div className="flex gap-2">
        <Button disabled={pending}>{submitLabel}</Button>
        {onCancel ? (
          <Button type="button" variant="ghost" onClick={onCancel} disabled={pending}>
            Cancel
          </Button>
        ) : null}
      </div>
    </form>
  );
}

function IdentityFields({
  draft,
  catalog,
  setDraft,
  notes,
}: {
  draft: IdentityDraft;
  catalog: IdentityCatalog;
  setDraft: (next: IdentityDraft) => void;
  notes: ReactNode;
}) {
  function set<Key extends keyof IdentityDraft>(key: Key, value: IdentityDraft[Key]) {
    setDraft({ ...draft, [key]: value });
  }

  const phone = isMobile(catalog, draft.os);
  const device = phone ? deviceNamed(catalog, draft.device) : undefined;
  const gpus = gpusFor(catalog, draft.os, draft.device);
  const screenValue = `${draft.logical_width}×${draft.logical_height}`;

  return (
    <div className="space-y-4 rounded-md border border-line p-3">
      <p className="nr-measure text-caption text-fg-muted">
        These fields bind: an operating system fixes the platform and narrows the releases,
        browsers and cards under it. The agreement between them is what a detector reads.
      </p>

      {notes}

      <Group title="Device">
        <ChoiceField
          label="Operating system"
          value={draft.os}
          options={catalog.operating_systems.map((entry) => entry.os)}
          onChange={(value) => setDraft(withOS(draft, catalog, value))}
        />
        {phone ? (
          <ChoiceField
            label="Handset"
            value={draft.device}
            options={devicesFor(catalog, draft.os).map((entry) => entry.name)}
            onChange={(value) => setDraft(withDevice(draft, catalog, value))}
            hint="Fixes the screen, card, cores and memory together."
          />
        ) : null}
        <ChoiceField
          label="Release"
          value={draft.os_version}
          options={osVersionsFor(catalog, draft.os, draft.device)}
          onChange={(value) => setDraft(withOSVersion(draft, catalog, value))}
          hint={`Reported to UA-CH as ${draft.platform_version || "—"}.`}
        />
        <ChoiceField
          label="Platform version"
          value={draft.platform_version}
          options={platformVersionsFor(catalog, draft.os, draft.os_version, draft.device)}
          onChange={(value) => set("platform_version", value)}
        />
        {phone ? (
          <ChoiceField
            label="Device model"
            value={draft.device_model}
            options={device?.models ?? []}
            onChange={(value) => set("device_model", value)}
            hint="What Sec-CH-UA-Model reports."
          />
        ) : (
          <TextField
            label="Device model"
            value={draft.device_model}
            onChange={(value) => set("device_model", value)}
            hint="Empty means desktop or laptop."
          />
        )}
        {phone ? (
          <ChoiceField
            label="navigator.platform"
            value={draft.navigator_platform}
            options={device?.navigator_platforms ?? []}
            onChange={(value) => set("navigator_platform", value)}
          />
        ) : (
          <>
            <ReadOnlyField label="navigator.platform" value={draft.navigator_platform} />
            <ReadOnlyField
              label="Bitness / architecture"
              value={[draft.bitness, draft.architecture].filter(Boolean).join(" · ")}
            />
          </>
        )}
      </Group>

      <Group title="Browser claim">
        <ChoiceField
          label="Browser"
          value={draft.browser}
          options={browsersFor(catalog, draft.os)}
          onChange={(value) =>
            setDraft(withBrowser(draft, catalog, value as IdentityDraft["browser"]))
          }
          hint="What it claims to be, not what runs."
        />
        <ChoiceField
          label="Browser version"
          value={draft.browser_version}
          options={browserVersionsFor(catalog, draft.browser)}
          onChange={(value) => set("browser_version", value)}
        />
      </Group>

      <Group title="Display">
        {phone ? (
          <>
            <ReadOnlyField label="Screen" value={screenValue} hint="The handset's panel." />
            <ReadOnlyField label="Device pixel ratio" value={draft.density_pixel_ratio} />
          </>
        ) : (
          <>
            <ChoiceField
              label="Screen"
              value={screenValue}
              options={catalog.screens.map((screen) => `${screen.width}×${screen.height}`)}
              onChange={(value) => {
                const [width, height] = value.split("×").map(Number);
                setDraft(withScreen(draft, width, height));
              }}
              hint="Ordered by how many desktops report it."
            />
            <ChoiceField
              label="Device pixel ratio"
              value={draft.density_pixel_ratio}
              options={catalog.density_pixel_ratios.map(String)}
              onChange={(value) => setDraft(withRatio(draft, value))}
            />
          </>
        )}
        <ReadOnlyField
          label="Physical pixels"
          value={`${draft.original_width}×${draft.original_height}`}
          hint="Logical size times the ratio."
        />
      </Group>

      <Group title="Hardware">
        <ChoiceField
          label="hardwareConcurrency"
          value={draft.hardware_concurrency}
          options={(device?.hardware_concurrency ?? catalog.hardware_concurrency).map(String)}
          onChange={(value) => set("hardware_concurrency", value)}
        />
        <ChoiceField
          label="deviceMemory (GiB)"
          value={draft.memory}
          options={(device?.memory ?? catalog.memory).map(String)}
          onChange={(value) => set("memory", value)}
          hint="Browsers report 8 at most."
        />
        <GPUField
          gpus={gpus}
          renderer={draft.webgl_renderer}
          onChange={(gpu) => setDraft(withGPU(draft, gpu))}
        />
        <ReadOnlyField label="WebGL vendor" value={draft.webgl_vendor} />
        <div className="flex flex-wrap items-end gap-2 pb-0.5">
          <ToggleField
            label="Battery"
            checked={draft.has_battery}
            onChange={(on) => set("has_battery", on)}
          />
          <ToggleField
            label="Mouse"
            checked={draft.has_mouse}
            onChange={(on) => set("has_mouse", on)}
          />
          <ToggleField
            label="Touch"
            checked={draft.has_touch}
            onChange={(on) => set("has_touch", on)}
          />
        </div>
      </Group>

      <Group title="Locale and network">
        <ChoiceField
          label="Geography"
          value={draft.geo}
          options={catalog.geos.map((geo) => geo.code)}
          onChange={(value) => setDraft(withGeo(draft, catalog, value))}
          hint="Fills the languages and clock that belong to it."
        />
        <TextField
          label="Languages"
          value={draft.language}
          onChange={(value) => set("language", value)}
          placeholder="en-US, en"
          required
        />
        <TextField
          label="Timezone"
          value={draft.timezone}
          onChange={(value) => set("timezone", value)}
          placeholder="Europe/Berlin"
          hint="IANA name. Empty resolves through the proxy."
        />
        <TextField
          label="Proxy URL"
          value={draft.proxy}
          onChange={(value) => set("proxy", value)}
          placeholder="http://user:pass@host:port"
          hint="A credential. Stored write-only and never shown again."
        />
        <TextField
          label="History length"
          value={draft.history_count}
          onChange={(value) => set("history_count", value)}
          hint="window.history.length. Empty leaves it alone."
        />
      </Group>
    </div>
  );
}

/**
 * A thousand renderer strings, filtered to the ones this platform can report.
 * A list this long is only usable if it is searchable, and only honest if it is
 * bound: Direct3D on a Mac is a contradiction, not an option.
 */
function GPUField({
  gpus,
  renderer,
  onChange,
}: {
  gpus: CatalogGPU[];
  renderer: string;
  onChange: (gpu: CatalogGPU) => void;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();

  return (
    <div className="space-y-1.5 sm:col-span-2">
      <Label htmlFor={id}>WebGL renderer</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            id={id}
            type="button"
            variant="secondary"
            role="combobox"
            aria-expanded={open}
            className="w-full justify-between font-normal"
          >
            <span className="truncate">{renderer || "Select a renderer"}</span>
            <ChevronsUpDown aria-hidden className="size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
          <Command>
            <CommandInput placeholder="Search renderers…" />
            <CommandList>
              <CommandEmpty>No renderer for this platform.</CommandEmpty>
              {gpus.map((gpu) => (
                <CommandItem
                  key={gpu.webgl_renderer}
                  value={gpu.webgl_renderer}
                  onSelect={() => {
                    onChange(gpu);
                    setOpen(false);
                  }}
                >
                  <Check
                    aria-hidden
                    className={
                      gpu.webgl_renderer === renderer ? "size-4 opacity-100" : "size-4 opacity-0"
                    }
                  />
                  <span className="truncate">{gpu.webgl_renderer}</span>
                </CommandItem>
              ))}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      <Hint>{gpus.length} cards report on this platform.</Hint>
    </div>
  );
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="space-y-2">
      <legend className="nr-label mb-2">{title}</legend>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
    </fieldset>
  );
}

function Hint({ children }: { children: ReactNode }) {
  return <p className="text-micro text-fg-muted">{children}</p>;
}

function TextField({
  label,
  value,
  onChange,
  required = false,
  placeholder,
  hint,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  placeholder?: string;
  hint?: ReactNode;
}) {
  const id = useId();
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required={required}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
      />
      {hint ? <Hint>{hint}</Hint> : null}
    </div>
  );
}

/** Fixed by something else on the form. Shown because the record carries it. */
function ReadOnlyField({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: ReactNode;
}) {
  const id = useId();
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} value={value} readOnly tabIndex={-1} className="text-fg-muted" />
      {hint ? <Hint>{hint}</Hint> : null}
    </div>
  );
}

/**
 * A catalogue-backed choice. A value the catalogue does not list is kept and
 * offered anyway — an older profile is not silently rewritten by opening it.
 */
function ChoiceField({
  label,
  value,
  options,
  onChange,
  hint,
}: {
  label: string;
  value: string;
  options: readonly string[];
  onChange: (value: string) => void;
  hint?: ReactNode;
}) {
  const id = useId();
  const listed = value === "" || options.includes(value) ? options : [value, ...options];

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {listed.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {hint ? <Hint>{hint}</Hint> : null}
    </div>
  );
}

function ToggleField({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const id = useId();
  return (
    <div className="flex h-9 items-center gap-2 rounded-md border border-input px-3">
      <Switch id={id} size="sm" checked={checked} onCheckedChange={onChange} />
      <Label htmlFor={id} className="text-caption text-fg-secondary">
        {label}
      </Label>
    </div>
  );
}
