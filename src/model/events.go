package model

type ProgressEvent struct {
	AssetID  string
	Filename string
	// Index/Total mark a countable batch position; when Total > 0 the emitter
	// renders the event as a live "[i/total] p%" counter line.
	Index int
	Total int
	Step  string
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
