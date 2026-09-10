package auth

import (
	"encoding/json"
	"io"
)

// jsonDecoder returns a strict JSON decoder (rejects unknown fields and trailing data).
func jsonDecoder(r io.Reader) *json.Decoder {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec
}
