package function

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSchemaValidateJSONSubset(t *testing.T) {
	t.Parallel()
	schema := Schema{
		Type:     TypeObject,
		Required: []string{"name", "scores", "active", "meta"},
		Properties: map[string]Schema{
			"name": {
				Type: TypeString, MinLength: Int(2), MaxLength: Int(4),
				Enum: []any{"éé", "test"},
			},
			"scores": {
				Type: TypeArray,
				Items: &Schema{
					Type: TypeInteger, Minimum: Number(0), Maximum: Number(10),
				},
			},
			"active": {Type: TypeBoolean},
			"meta":   {Type: TypeNull},
			"ratio":  {Type: TypeNumber, Minimum: Number(0.25), Maximum: Number(0.75)},
		},
		AdditionalProperties: Bool(false),
	}

	if err := schema.ValidateJSON(json.RawMessage(
		`{"name":"éé","scores":[0,5,10],"active":true,"meta":null,"ratio":0.5}`,
	)); err != nil {
		t.Fatalf("valid value rejected: %v", err)
	}

	err := schema.ValidateJSON(json.RawMessage(
		`{"name":"x","scores":[1.5,11],"active":"yes","extra":1}`,
	))
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("expected schema mismatch, got %v", err)
	}
	message := err.Error()
	for _, want := range []string{
		"$.meta: required property is missing",
		"$.name: value is not one of the allowed values",
		"$.name: length 1 is less than minimum 2",
		"$.scores[0]: expected integer",
		"$.scores[1]: number exceeds maximum 10",
		"$.active: expected boolean",
		"$.extra: additional property is not allowed",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}

func TestSchemaJSONTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		schema  Schema
		valid   string
		invalid string
	}{
		{"object", Schema{Type: TypeObject}, `{}`, `[]`},
		{"array", Schema{Type: TypeArray}, `[]`, `{}`},
		{"string", Schema{Type: TypeString}, `""`, `1`},
		{"number", Schema{Type: TypeNumber}, `1.25`, `"1.25"`},
		{"integer", Schema{Type: TypeInteger}, `1e3`, `1.01`},
		{"boolean", Schema{Type: TypeBoolean}, `false`, `null`},
		{"null", Schema{Type: TypeNull}, `null`, `false`},
		{"anything", Schema{}, `{"nested":[null,1,"x"]}`, ``},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.schema.ValidateJSON(json.RawMessage(test.valid)); err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
			err := test.schema.ValidateJSON(json.RawMessage(test.invalid))
			if test.name == "anything" {
				if !errors.Is(err, ErrInvalidJSON) {
					t.Fatalf("expected invalid JSON, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrSchemaMismatch) {
				t.Fatalf("expected mismatch, got %v", err)
			}
		})
	}
}

func TestSchemaRejectsMalformedJSONAndTrailingValues(t *testing.T) {
	t.Parallel()
	schema := Schema{}
	for _, input := range []string{"", "{", "null true"} {
		if err := schema.ValidateJSON(json.RawMessage(input)); !errors.Is(err, ErrInvalidJSON) {
			t.Errorf("input %q: expected invalid JSON, got %v", input, err)
		}
	}
}

func TestSchemaDefinitionValidation(t *testing.T) {
	t.Parallel()
	tests := []Schema{
		{Type: "date"},
		{Type: TypeString, Minimum: Number(1)},
		{Type: TypeArray, Properties: map[string]Schema{"x": {}}},
		{Type: TypeObject, Required: []string{"missing"}},
		{Type: TypeObject, Required: []string{"x", "x"}, Properties: map[string]Schema{"x": {}}},
		{Type: TypeString, MinLength: Int(4), MaxLength: Int(1)},
		{Type: TypeNumber, Minimum: Number(2), Maximum: Number(1)},
		{Enum: []any{}},
		{Enum: []any{"same", "same"}},
	}
	for i, schema := range tests {
		if err := schema.ValidateDefinition(); !errors.Is(err, ErrInvalidSchema) {
			t.Errorf("case %d: expected invalid schema, got %v", i, err)
		}
	}
}

func TestDecodeSchemaIsStrict(t *testing.T) {
	t.Parallel()
	if _, err := DecodeSchema(json.RawMessage(`{"type":"string","pattern":"x"}`)); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected unsupported field rejection, got %v", err)
	}
	schema, err := DecodeSchema(json.RawMessage(
		`{"type":"array","items":{"type":"integer","minimum":1}}`,
	))
	if err != nil {
		t.Fatalf("decode valid schema: %v", err)
	}
	if err := schema.ValidateJSON(json.RawMessage(`[1,2,3]`)); err != nil {
		t.Fatalf("decoded schema rejected value: %v", err)
	}
}

func TestSchemaEnumUsesCanonicalJSONEquality(t *testing.T) {
	t.Parallel()
	schema := Schema{Enum: []any{
		map[string]any{"b": json.Number("2"), "a": json.Number("1")},
	}}
	if err := schema.ValidateJSON(json.RawMessage(`{"a":1,"b":2}`)); err != nil {
		t.Fatalf("canonical object enum did not match: %v", err)
	}
}
