package model

type ProgressEvent struct {
	AssetID  string
	Filename string
	Step     string
}

type DiffSymbol string

const (
	DiffAdd    DiffSymbol = "+"
	DiffChange DiffSymbol = "~"
)

type DiffEntry struct {
	Tag    string
	Symbol DiffSymbol
	Old    string
	New    string
}

type DiffEvent struct {
	AssetID  string
	Filename string
	Index    int
	Total    int
	Entries  []DiffEntry
}

type AllDoneEvent struct {
	Results []ProcessResult
}

type DiffAction int

const (
	ActionConfirm DiffAction = iota
	ActionSkip
	ActionQuit
)

type EventEmitter interface {
	EmitProgress(event ProgressEvent)
	EmitDiff(event DiffEvent) DiffAction
	EmitAllDone(event AllDoneEvent)
}
