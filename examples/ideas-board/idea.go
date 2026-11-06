package main

// tag::entity[]

// Idea is a single entry on the ideas board. Many users upvote the same row
// concurrently, which is what makes Votes a contention hotspot.
type Idea struct {
	ID    string // stable identifier (a string/UUID)
	Text  string // the idea itself
	Votes int    // running upvote count — the contended column
}

// end::entity[]
