package model

// FieldDiff is a single field that differs between two matched objects.
// From/To hold the raw values (so JSON stays typed and renderers can format
// them); an absent side is nil.
type FieldDiff struct {
	Path string `json:"path"`
	From any    `json:"from"`
	To   any    `json:"to"`
}
