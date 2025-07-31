package observability

import "go.opentelemetry.io/otel/attribute"

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
