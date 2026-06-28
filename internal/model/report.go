package model

// ResourceRef identifies a resource by kind and name.
type ResourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ResourceDiff is a matched resource whose content differs, with its fields.
type ResourceDiff struct {
	Kind   string      `json:"kind"`
	Name   string      `json:"name"`
	Fields []FieldDiff `json:"fields"`
}

// DiffReport is the full structured result of comparing two environments. It is
// the single value all renderers (text, json, markdown) consume.
type DiffReport struct {
	From     string         `json:"from"`
	To       string         `json:"to"`
	OnlyFrom []ResourceRef  `json:"onlyFrom"`
	OnlyTo   []ResourceRef  `json:"onlyTo"`
	Differs  []ResourceDiff `json:"differs"`
	Same     int            `json:"same"`
	Warnings []string       `json:"warnings,omitempty"`
}
