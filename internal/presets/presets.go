package presets

var Presets = map[string]map[string]string{
	"coder": {
		"contents":      "write",
		"pull_requests": "write",
		"issues":        "read",
		"metadata":      "read",
	},
	"reviewer": {
		"contents":      "read",
		"pull_requests": "write",
		"issues":        "write",
		"metadata":      "read",
	},
	"docs": {
		"contents": "write",
		"pages":    "write",
		"metadata": "read",
	},
	"ci": {
		"contents": "read",
		"checks":   "write",
		"actions":  "write",
		"metadata": "read",
	},
	"triage": {
		"issues":        "write",
		"pull_requests": "write",
		"metadata":      "read",
	},
	"minimal": {
		"contents": "read",
		"metadata": "read",
	},
}

func Names() []string {
	return []string{"coder", "reviewer", "docs", "ci", "triage", "minimal"}
}
