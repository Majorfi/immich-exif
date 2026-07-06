package model

type ProgressEvent struct {
	AssetID  string
	Filename string
	// Index/Total mark a countable batch position; when Total > 0 the emitter
	// renders the event as a live "[i/total] <step>" counter line.
	Index int
	Total int
	// Percent is the progress of the current step (e.g. download transfer),
	// not of the batch; 0 means no percentage is shown.
	Percent int
	// Done marks the end of this asset's processing so the emitter can retire
	// its live counter line.
	Done bool
	Step string
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
