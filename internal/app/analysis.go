package app

import (
	"github.com/ViktorBarzin/agentmd/internal/discover"
	"github.com/ViktorBarzin/agentmd/internal/model"
)

// attachAnalysis marks each context with its last analysis, if any.
func (a *App) attachAnalysis(res *discover.Result, contexts []model.Context) {}

// analysisFindings returns cached analysis findings for the contexts.
func (a *App) analysisFindings(contexts []model.Context) []model.Finding { return nil }
