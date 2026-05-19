package validation

import (
	"testing"
)

// TestValidation tests basic validation functionality
func TestValidation(t *testing.T) {
	// Create a simple schema
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
			"age":  map[string]interface{}{"type": "integer"},
		},
		"required": []string{"name"},
	}

	cache := NewSchemaCache()

	// Test 1: Valid args
	validArgs := `{"name": "Test", "age": 30}`
	valErr := Validate("test-tool", schema, validArgs, cache)
	if valErr != nil {
		t.Errorf("Expected no validation error, got: %v", valErr)
	}

	// Test 2: Missing required field
	missingRequiredArgs := `{"age": 30}`
	valErr = Validate("test-tool", schema, missingRequiredArgs, cache)
	if valErr == nil {
		t.Error("Expected validation error for missing required field, got nil")
	}

	// Test 3: Invalid type for age
	invalidTypeArgs := `{"name": "Test", "age": "thirty"}`
	valErr = Validate("test-tool", schema, invalidTypeArgs, cache)
	if valErr == nil {
		t.Error("Expected validation error for invalid type, got nil")
	}

	// Test 4: Empty args
	emptyArgs := ""
	valErr = Validate("test-tool", schema, emptyArgs, nil)
	if valErr == nil {
		t.Error("Expected validation error for missing required field, got nil")
	}

	// Test 5: Nil schema skips validation
	valErr = Validate("test-tool", nil, validArgs, cache)
	if valErr != nil {
		t.Errorf("Nil schema should skip validation, but got error: %v", valErr)
	}
}
