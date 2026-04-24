package interfaces

const DefaultAgentName = "main"

type ManagerStatus int

const (
	StatusIdle ManagerStatus = iota
	StatusRunning
	StatusStopped
	StatusError
)
