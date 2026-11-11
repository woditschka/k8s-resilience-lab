package scenario

// Following reads a held cluster's Timeline one entry at a time, after the
// Timeline recorded so far, for the lines its terminal prints. It reads through
// the Wait narratives' own reading, so each line is worded as a narrative's line
// for the same change, and carries no indent.
type Following struct {
	narrating *narration
	// unfinished holds the moments read but not yet complete, in reading order.
	unfinished []*moment
}

// NewFollowing reads the Timeline so far silently and discards its moments, as
// a Wait does, so a Pod scheduled before the hold reads its Running as a moment
// of its own.
func NewFollowing(soFar Timeline) *Following {
	narrating := newNarration(soFar.deploymentsNamed())
	for _, entry := range soFar.entries {
		narrating.read(entry)
	}
	narrating.cut()

	return &Following{narrating: narrating}
}

// Read gives the lines one later entry completes: a When's line, or a clicked
// Failure's named not injected in its place, and each moment complete with it,
// a printed moment never printed again. Most give none.
func (f *Following) Read(entry Entry) []string {
	if entry.notesAWhen() || entry.notesAFailureNotInjected() {
		return []string{entry.Detail}
	}

	f.narrating.read(entry)
	f.unfinished = append(f.unfinished, f.narrating.taken()...)

	return f.completed()
}

// completed keeps the reading order, so a Pod's scheduling completed by its
// eviction prints before the eviction.
func (f *Following) completed() []string {
	var lines []string
	unfinished := f.unfinished[:0]
	for _, read := range f.unfinished {
		if read.complete() {
			lines = append(lines, read.line())
		} else {
			unfinished = append(unfinished, read)
		}
	}
	f.unfinished = unfinished

	return lines
}
