package model

// FieldDiff is a single field that differs between two matched objects.
// From/To hold the rendered values; an absent side is rendered as "<none>".
type FieldDiff struct {
	Path string
	From string
	To   string
}
