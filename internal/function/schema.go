package function

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// JSONType is one of the JSON value types supported by the atomic-function
// contract validator. The deliberately small subset keeps validation behavior
// identical in the control plane and in agents.
type JSONType string

const (
	TypeObject  JSONType = "object"
	TypeArray   JSONType = "array"
	TypeString  JSONType = "string"
	TypeNumber  JSONType = "number"
	TypeInteger JSONType = "integer"
	TypeBoolean JSONType = "boolean"
	TypeNull    JSONType = "null"
)

var (
	// ErrInvalidSchema identifies an invalid schema definition.
	ErrInvalidSchema = errors.New("invalid JSON schema")
	// ErrSchemaMismatch identifies JSON data that does not satisfy a schema.
	ErrSchemaMismatch = errors.New("JSON schema mismatch")
	// ErrInvalidJSON identifies malformed or trailing JSON input.
	ErrInvalidJSON = errors.New("invalid JSON")
)

// Schema is the JSON Schema subset supported for built-in function contracts.
// A zero Schema accepts every valid JSON value.
type Schema struct {
	Type                 JSONType          `json:"type,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	AdditionalProperties *bool             `json:"additionalProperties,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	Enum                 []any             `json:"enum,omitempty"`
	Minimum              *float64          `json:"minimum,omitempty"`
	Maximum              *float64          `json:"maximum,omitempty"`
	MinLength            *int              `json:"minLength,omitempty"`
	MaxLength            *int              `json:"maxLength,omitempty"`
}

// ValidationError describes one deterministic contract violation.
type ValidationError struct {
	Path    string `json:"path"`
	Keyword string `json:"keyword"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationErrors contains all violations found in a value.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ErrSchemaMismatch.Error()
	}
	parts := make([]string, len(e))
	for i := range e {
		parts[i] = e[i].Error()
	}
	return strings.Join(parts, "; ")
}

func (e ValidationErrors) Unwrap() error {
	return ErrSchemaMismatch
}

// Bool returns a pointer suitable for AdditionalProperties.
func Bool(value bool) *bool {
	return &value
}

// Int returns a pointer suitable for length constraints.
func Int(value int) *int {
	return &value
}

// Number returns a pointer suitable for numeric constraints.
func Number(value float64) *float64 {
	return &value
}

// DecodeSchema strictly decodes and validates a schema definition.
func DecodeSchema(raw json.RawMessage) (Schema, error) {
	var schema Schema
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schema); err != nil {
		return Schema{}, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	if err := requireEOF(decoder); err != nil {
		return Schema{}, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	if err := schema.ValidateDefinition(); err != nil {
		return Schema{}, err
	}
	return schema, nil
}

// ValidateDefinition verifies that the schema only uses the supported subset
// and that its constraints are internally consistent.
func (s Schema) ValidateDefinition() error {
	var problems []string
	s.validateDefinition("$", 0, &problems)
	if len(problems) != 0 {
		return fmt.Errorf("%w: %s", ErrInvalidSchema, strings.Join(problems, "; "))
	}
	return nil
}

func (s Schema) validateDefinition(path string, depth int, problems *[]string) {
	if depth > 100 {
		*problems = append(*problems, path+": schema nesting exceeds 100 levels")
		return
	}
	switch s.Type {
	case "", TypeObject, TypeArray, TypeString, TypeNumber, TypeInteger, TypeBoolean, TypeNull:
	default:
		*problems = append(*problems, fmt.Sprintf("%s.type: unsupported type %q", path, s.Type))
	}

	if len(s.Required) != 0 || len(s.Properties) != 0 || s.AdditionalProperties != nil {
		if s.Type != TypeObject {
			*problems = append(*problems, path+": object keywords require type object")
		}
	}
	if s.Items != nil && s.Type != TypeArray {
		*problems = append(*problems, path+": items requires type array")
	}
	if (s.Minimum != nil || s.Maximum != nil) && s.Type != TypeNumber && s.Type != TypeInteger {
		*problems = append(*problems, path+": minimum/maximum require type number or integer")
	}
	if (s.MinLength != nil || s.MaxLength != nil) && s.Type != TypeString {
		*problems = append(*problems, path+": minLength/maxLength require type string")
	}

	seenRequired := make(map[string]struct{}, len(s.Required))
	for _, name := range s.Required {
		if name == "" {
			*problems = append(*problems, path+".required: property name cannot be empty")
			continue
		}
		if _, ok := seenRequired[name]; ok {
			*problems = append(*problems, fmt.Sprintf("%s.required: duplicate property %q", path, name))
		}
		seenRequired[name] = struct{}{}
		if _, ok := s.Properties[name]; !ok {
			*problems = append(*problems, fmt.Sprintf("%s.required: property %q is not declared", path, name))
		}
	}

	if s.Minimum != nil && (math.IsNaN(*s.Minimum) || math.IsInf(*s.Minimum, 0)) {
		*problems = append(*problems, path+".minimum: must be finite")
	}
	if s.Maximum != nil && (math.IsNaN(*s.Maximum) || math.IsInf(*s.Maximum, 0)) {
		*problems = append(*problems, path+".maximum: must be finite")
	}
	if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
		*problems = append(*problems, path+": minimum exceeds maximum")
	}
	if s.MinLength != nil && *s.MinLength < 0 {
		*problems = append(*problems, path+".minLength: cannot be negative")
	}
	if s.MaxLength != nil && *s.MaxLength < 0 {
		*problems = append(*problems, path+".maxLength: cannot be negative")
	}
	if s.MinLength != nil && s.MaxLength != nil && *s.MinLength > *s.MaxLength {
		*problems = append(*problems, path+": minLength exceeds maxLength")
	}

	if s.Enum != nil {
		if len(s.Enum) == 0 {
			*problems = append(*problems, path+".enum: must contain at least one value")
		}
		seen := make(map[string]struct{}, len(s.Enum))
		for i, value := range s.Enum {
			encoded, err := canonicalJSON(value)
			if err != nil {
				*problems = append(*problems, fmt.Sprintf("%s.enum[%d]: %v", path, i, err))
				continue
			}
			key := string(encoded)
			if _, ok := seen[key]; ok {
				*problems = append(*problems, fmt.Sprintf("%s.enum: duplicate value at index %d", path, i))
			}
			seen[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		child := s.Properties[name]
		child.validateDefinition(propertyPath(path, name), depth+1, problems)
	}
	if s.Items != nil {
		s.Items.validateDefinition(path+"[]", depth+1, problems)
	}
}

// ValidateJSON decodes JSON without losing number precision and validates it.
func (s Schema) ValidateJSON(raw json.RawMessage) error {
	value, err := decodeJSON(raw)
	if err != nil {
		return err
	}
	return s.Validate(value)
}

// Validate validates a decoded JSON-compatible value.
func (s Schema) Validate(value any) error {
	if err := s.ValidateDefinition(); err != nil {
		return err
	}
	var violations ValidationErrors
	s.validateValue(value, "$", 0, &violations)
	if len(violations) != 0 {
		return violations
	}
	return nil
}

func (s Schema) validateValue(value any, path string, depth int, violations *ValidationErrors) {
	if depth > 100 {
		*violations = append(*violations, ValidationError{
			Path: path, Keyword: "depth", Message: "value nesting exceeds 100 levels",
		})
		return
	}

	if s.Type != "" && !matchesType(value, s.Type) {
		*violations = append(*violations, ValidationError{
			Path: path, Keyword: "type",
			Message: fmt.Sprintf("expected %s, got %s", s.Type, jsonTypeOf(value)),
		})
		return
	}

	if s.Enum != nil {
		matched := false
		actual, err := canonicalJSON(value)
		if err == nil {
			for _, candidate := range s.Enum {
				expected, candidateErr := canonicalJSON(candidate)
				if candidateErr == nil && bytes.Equal(actual, expected) {
					matched = true
					break
				}
			}
		}
		if !matched {
			*violations = append(*violations, ValidationError{
				Path: path, Keyword: "enum", Message: "value is not one of the allowed values",
			})
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		for _, required := range s.Required {
			if _, ok := typed[required]; !ok {
				*violations = append(*violations, ValidationError{
					Path: propertyPath(path, required), Keyword: "required", Message: "required property is missing",
				})
			}
		}
		keys := make([]string, 0, len(typed))
		for name := range typed {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		allowAdditional := s.AdditionalProperties == nil || *s.AdditionalProperties
		for _, name := range keys {
			child, declared := s.Properties[name]
			if declared {
				child.validateValue(typed[name], propertyPath(path, name), depth+1, violations)
				continue
			}
			if !allowAdditional {
				*violations = append(*violations, ValidationError{
					Path: propertyPath(path, name), Keyword: "additionalProperties",
					Message: "additional property is not allowed",
				})
			}
		}
	case []any:
		if s.Items != nil {
			for i := range typed {
				s.Items.validateValue(typed[i], fmt.Sprintf("%s[%d]", path, i), depth+1, violations)
			}
		}
	case string:
		length := utf8.RuneCountInString(typed)
		if s.MinLength != nil && length < *s.MinLength {
			*violations = append(*violations, ValidationError{
				Path: path, Keyword: "minLength",
				Message: fmt.Sprintf("length %d is less than minimum %d", length, *s.MinLength),
			})
		}
		if s.MaxLength != nil && length > *s.MaxLength {
			*violations = append(*violations, ValidationError{
				Path: path, Keyword: "maxLength",
				Message: fmt.Sprintf("length %d exceeds maximum %d", length, *s.MaxLength),
			})
		}
	default:
		if isJSONNumber(typed) && (s.Minimum != nil || s.Maximum != nil) {
			number, ok := numberRat(typed)
			if !ok {
				*violations = append(*violations, ValidationError{
					Path: path, Keyword: "type", Message: "number is not finite",
				})
				return
			}
			if s.Minimum != nil && number.Cmp(ratFromFloat(*s.Minimum)) < 0 {
				*violations = append(*violations, ValidationError{
					Path: path, Keyword: "minimum",
					Message: fmt.Sprintf("number is less than minimum %s", strconv.FormatFloat(*s.Minimum, 'g', -1, 64)),
				})
			}
			if s.Maximum != nil && number.Cmp(ratFromFloat(*s.Maximum)) > 0 {
				*violations = append(*violations, ValidationError{
					Path: path, Keyword: "maximum",
					Message: fmt.Sprintf("number exceeds maximum %s", strconv.FormatFloat(*s.Maximum, 'g', -1, 64)),
				})
			}
		}
	}
}

func decodeJSON(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrInvalidJSON)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if err := requireEOF(decoder); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return value, nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func matchesType(value any, expected JSONType) bool {
	switch expected {
	case TypeObject:
		_, ok := value.(map[string]any)
		return ok
	case TypeArray:
		_, ok := value.([]any)
		return ok
	case TypeString:
		_, ok := value.(string)
		return ok
	case TypeNumber:
		return isJSONNumber(value)
	case TypeInteger:
		number, ok := numberRat(value)
		return ok && number.IsInt()
	case TypeBoolean:
		_, ok := value.(bool)
		return ok
	case TypeNull:
		return value == nil
	default:
		return true
	}
}

func jsonTypeOf(value any) string {
	switch value.(type) {
	case nil:
		return string(TypeNull)
	case map[string]any:
		return string(TypeObject)
	case []any:
		return string(TypeArray)
	case string:
		return string(TypeString)
	case bool:
		return string(TypeBoolean)
	default:
		if number, ok := numberRat(value); ok {
			if number.IsInt() {
				return string(TypeInteger)
			}
			return string(TypeNumber)
		}
		return fmt.Sprintf("%T", value)
	}
}

func isJSONNumber(value any) bool {
	_, ok := numberRat(value)
	return ok
}

func numberRat(value any) (*big.Rat, bool) {
	switch typed := value.(type) {
	case json.Number:
		number, ok := new(big.Rat).SetString(typed.String())
		return number, ok
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, false
		}
		return ratFromFloat(typed), true
	case float32:
		value64 := float64(typed)
		if math.IsNaN(value64) || math.IsInf(value64, 0) {
			return nil, false
		}
		return ratFromFloat(value64), true
	case int:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int8:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int16:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int32:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int64:
		return new(big.Rat).SetInt64(typed), true
	case uint:
		return new(big.Rat).SetUint64(uint64(typed)), true
	case uint8:
		return new(big.Rat).SetUint64(uint64(typed)), true
	case uint16:
		return new(big.Rat).SetUint64(uint64(typed)), true
	case uint32:
		return new(big.Rat).SetUint64(uint64(typed)), true
	case uint64:
		return new(big.Rat).SetUint64(typed), true
	default:
		return nil, false
	}
}

func ratFromFloat(value float64) *big.Rat {
	rat, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'g', -1, 64))
	if !ok {
		return new(big.Rat)
	}
	return rat
}

func propertyPath(parent, property string) string {
	if isIdentifier(property) {
		return parent + "." + property
	}
	encoded, _ := json.Marshal(property)
	return parent + "[" + string(encoded) + "]"
}

func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func canonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeJSON(encoded)
	if err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}
