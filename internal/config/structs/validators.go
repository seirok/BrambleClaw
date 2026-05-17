package structs

import "neoclaw/internal/logger"

// ValidatePositiveInt validates that an integer is positive, otherwise uses default and logs a warning.
func ValidatePositiveInt(field *int, defaultValue int, fieldName string) {
	if *field <= 0 {
		logger.L().Warn().Int("invalid_"+fieldName, *field).Msgf("Invalid %s, using default", fieldName)
		*field = defaultValue
	}
}

// ValidateIntRange validates that an integer is within [min, max], otherwise uses default and logs a warning.
func ValidateIntRange(field *int, min, max, defaultValue int, fieldName string) {
	if *field < min || *field > max {
		logger.L().Warn().Int("invalid_"+fieldName, *field).Msgf("Invalid %s, using default", fieldName)
		*field = defaultValue
	}
}

// ValidateNonEmptyString validates that a string is not empty, otherwise uses default and optionally logs an error.
func ValidateNonEmptyString(field *string, defaultValue string, fieldName string, isRequired bool) {
	if *field == "" {
		if isRequired {
			logger.L().Error().Msgf("%s is required", fieldName)
		}
		*field = defaultValue
	}
}

// EnsureSlice ensures a slice is not nil.
func EnsureSlice[T any](field *[]T) {
	if *field == nil {
		*field = make([]T, 0)
	}
}
