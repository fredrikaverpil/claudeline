package main

import (
	"github.com/fredrikaverpil/pocket/pk"
	"github.com/fredrikaverpil/pocket/tasks/github"
	"github.com/fredrikaverpil/pocket/tasks/golang"
)

// Config is the Pocket configuration for this project.
// Edit this file to define your tasks and composition.
var Config = &pk.Config{
	Auto: pk.Serial(
		golang.Tasks(),
		pk.WithOptions(
			github.Tasks(),
			pk.WithFlag(github.Workflows, github.FlagIncludeGoreleaser, true),
		),
	),
}
