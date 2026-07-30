/**
 * Derive form fields from a function's published input schema.
 *
 * The generated form is a convenience over the contract, never a replacement
 * for it: anything the generator cannot represent faithfully (nested objects,
 * arrays, unconstrained values) falls back to a raw JSON editor for that field
 * rather than guessing at a control. The whole form also has a raw JSON escape
 * hatch, because the manifest is the authority and an operator must always be
 * able to send exactly what they mean.
 */
import type { JSONSchema } from "@/lib/api/types";
import { isSecretKey } from "./redaction";

export type FieldKind = "string" | "number" | "integer" | "boolean" | "enum" | "json";

export interface SchemaField {
  name: string;
  kind: FieldKind;
  required: boolean;
  /** Values for `kind === "enum"`. */
  options?: string[];
  description?: string;
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  /** The key name looks like a secret; the control masks its value. */
  secret: boolean;
}

interface SchemaLike {
  type?: string;
  required?: string[];
  properties?: Record<string, SchemaLike>;
  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  description?: string;
  items?: SchemaLike;
}

/**
 * Fields for a top-level object schema, or `null` when the schema is not an
 * object — in which case the caller should present the raw JSON editor alone.
 */
export function deriveFields(schema: JSONSchema | undefined): SchemaField[] | null {
  const root = schema as SchemaLike | undefined;
  if (!root || root.type !== "object" || !root.properties) return null;

  const required = new Set(root.required ?? []);

  return Object.entries(root.properties).map(([name, property]) => {
    const enumValues = property.enum?.filter(
      (value): value is string => typeof value === "string",
    );

    let kind: FieldKind = "json";
    if (enumValues && enumValues.length > 0) {
      kind = "enum";
    } else {
      switch (property.type) {
        case "string":
          kind = "string";
          break;
        case "number":
          kind = "number";
          break;
        case "integer":
          kind = "integer";
          break;
        case "boolean":
          kind = "boolean";
          break;
        default:
          // object, array, or unspecified: hand it to the raw editor.
          kind = "json";
      }
    }

    return {
      name,
      kind,
      required: required.has(name),
      options: enumValues,
      description: property.description,
      minimum: property.minimum,
      maximum: property.maximum,
      minLength: property.minLength,
      maxLength: property.maxLength,
      secret: isSecretKey(name),
    };
  });
}

/** A blank form state matching the derived fields. */
export function emptyValues(fields: SchemaField[]): Record<string, string> {
  return Object.fromEntries(fields.map((field) => [field.name, field.kind === "boolean" ? "false" : ""]));
}

export interface CoercionResult {
  input: Record<string, unknown>;
  errors: Record<string, string>;
}

/**
 * Turn string form state into a typed payload.
 *
 * Empty optional fields are omitted rather than sent as empty strings — the
 * server validates against the manifest, and a stray `""` would fail a
 * constraint the operator never intended to set.
 */
export function coerceValues(
  fields: SchemaField[],
  values: Record<string, string>,
): CoercionResult {
  const input: Record<string, unknown> = {};
  const errors: Record<string, string> = {};

  for (const field of fields) {
    const raw = values[field.name] ?? "";

    if (field.kind === "boolean") {
      if (raw === "" && !field.required) continue;
      input[field.name] = raw === "true";
      continue;
    }

    if (raw.trim() === "") {
      if (field.required) errors[field.name] = "Required.";
      continue;
    }

    switch (field.kind) {
      case "number":
      case "integer": {
        const parsed = Number(raw);
        if (Number.isNaN(parsed)) {
          errors[field.name] = "Must be a number.";
          break;
        }
        if (field.kind === "integer" && !Number.isInteger(parsed)) {
          errors[field.name] = "Must be a whole number.";
          break;
        }
        if (field.minimum !== undefined && parsed < field.minimum) {
          errors[field.name] = `Must be at least ${field.minimum}.`;
          break;
        }
        if (field.maximum !== undefined && parsed > field.maximum) {
          errors[field.name] = `Must be at most ${field.maximum}.`;
          break;
        }
        input[field.name] = parsed;
        break;
      }
      case "json": {
        try {
          input[field.name] = JSON.parse(raw);
        } catch {
          errors[field.name] = "Must be valid JSON.";
        }
        break;
      }
      default: {
        if (field.minLength !== undefined && raw.length < field.minLength) {
          errors[field.name] = `Must be at least ${field.minLength} characters.`;
          break;
        }
        if (field.maxLength !== undefined && raw.length > field.maxLength) {
          errors[field.name] = `Must be at most ${field.maxLength} characters.`;
          break;
        }
        input[field.name] = raw;
      }
    }
  }

  return { input, errors };
}
