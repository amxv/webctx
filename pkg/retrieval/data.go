package retrieval

import "github.com/amxv/webctx/internal/app"

type ResearchInput = app.ResearchDataInput
type InspectInput = app.InspectDataInput
type ExecuteInput = app.ExecuteDataInput
type DataCall = app.DataCall
type DataError = app.DataError
type MapInput = app.MapSiteInput

func Research(input ResearchInput) (map[string]any, error) { return app.ResearchData(input) }
func Inspect(input InspectInput) (map[string]any, error)   { return app.InspectData(input) }
func Execute(input ExecuteInput) (map[string]any, error)   { return app.ExecuteData(input) }
func DataErrorResult(err error) map[string]any             { return app.DataErrorResult(err) }
func MapSitePage(input MapInput) (map[string]any, error)   { return app.MapSitePage(input) }
