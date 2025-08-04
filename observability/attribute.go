package observability

import (
	"fmt"
	"go.opentelemetry.io/otel/attribute"
)

// String creates a new key-value pair with a string value.
func String(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

// Int creates a new key-value pair with an integer value.
func Int(key string, value int) attribute.KeyValue {
	return attribute.Int(key, value)
}

// Bool creates a new key-value pair with a boolean value.
func Bool(key string, value bool) attribute.KeyValue {
	return attribute.Bool(key, value)
}

// ToAttribute converts a key and an interface{} value to an OpenTelemetry attribute.KeyValue.
// It handles common types and provides a safe fallback for others.
func ToAttribute(k string, v interface{}) attribute.KeyValue {
	switch val := v.(type) {
	case string:
		return attribute.String(k, val)
	case int:
		return attribute.Int(k, val)
	case int64:
		return attribute.Int64(k, val)
	case bool:
		return attribute.Bool(k, val)
	case float64:
		return attribute.Float64(k, val)
	default:
		return attribute.String(k, fmt.Sprintf("%v", v))
	}
}
