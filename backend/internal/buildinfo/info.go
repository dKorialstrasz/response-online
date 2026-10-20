package buildinfo

// These values are replaced by linker flags for immutable build artifacts.
var (
	Commit  = "development"
	BuiltAt = "unknown"
)

const (
	ProtocolVersion = "1.2"
	ContentVersion  = "base-cards-v1"
	RulesVersion    = "response-rules-2026-10-17"
	EngineVersion   = "base-abilities-v2"
)

type Info struct {
	Commit          string `json:"commit"`
	BuiltAt         string `json:"builtAt"`
	ProtocolVersion string `json:"protocolVersion"`
	ContentVersion  string `json:"contentVersion"`
	RulesVersion    string `json:"rulesVersion"`
	EngineVersion   string `json:"engineVersion"`
}

func Current() Info {
	return Info{
		Commit:          Commit,
		BuiltAt:         BuiltAt,
		ProtocolVersion: ProtocolVersion,
		ContentVersion:  ContentVersion,
		RulesVersion:    RulesVersion,
		EngineVersion:   EngineVersion,
	}
}
