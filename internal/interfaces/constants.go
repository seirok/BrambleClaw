package interfaces

const DefaultAgentName = "main"
const CliChannelName = "cli"

type ManagerStatus int

const (
	StatusIdle ManagerStatus = iota
	StatusRunning
	StatusStopped
	StatusError
)

const (
	SessionSuffix = ".jsonl"
	MetaSuffix    = ".meta.json"
)
