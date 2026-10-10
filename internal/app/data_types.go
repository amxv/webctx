package app

// ResearchData discovers data sources and their available operations. Search
// and catalogue browsing are both free Firecrawl Alexandria operations.
type ResearchDataInput struct {
	Query      string   `json:"query,omitempty" jsonschema:"Optional natural-language description of the information needed"`
	Mode       string   `json:"mode,omitempty" jsonschema:"ranked semantic discovery or catalogue browsing with filters and pagination; defaults to ranked for a query"`
	URLs       []string `json:"urls,omitempty" jsonschema:"Filter by websites associated with a data source"`
	Sources    []string `json:"sources,omitempty" jsonschema:"Filter by source IDs returned by research"`
	Categories []string `json:"categories,omitempty" jsonschema:"Filter by catalogue category IDs"`
	Groups     []string `json:"groups,omitempty" jsonschema:"Filter by catalogue group IDs"`
	Operations []string `json:"operations,omitempty" jsonschema:"Filter by exact operation IDs"`
	View       string   `json:"view,omitempty" jsonschema:"What to browse: sources, groups, or operations; default operations for a query, otherwise sources"`
	Include    []string `json:"include,omitempty" jsonschema:"Additional contract details: inputs, output, examples. Default is a concise overview"`
	Limit      int      `json:"limit,omitempty" jsonschema:"Maximum results in this page, 1-100; default 10"`
	Offset     int      `json:"offset,omitempty" jsonschema:"Catalogue pagination offset, zero-based"`
	Raw        bool     `json:"raw,omitempty" jsonschema:"Also include the complete original Firecrawl response"`
}

// InspectDataInput returns the exact contract for a selected operation, or
// browses all operations of a source when passed just a source ID.
type InspectDataInput struct {
	ID      string   `json:"id" jsonschema:"Source or operation ID returned by research, e.g. particle/podcasts/episodes/search"`
	Include []string `json:"include,omitempty" jsonschema:"Contract sections: inputs, output, examples; all by default"`
	Raw     bool     `json:"raw,omitempty" jsonschema:"Include the complete original Firecrawl response"`
}

type DataCall struct {
	ID     string         `json:"id" jsonschema:"Exact operation ID returned by research or inspect"`
	Inputs map[string]any `json:"inputs,omitempty" jsonschema:"Provider-specific input fields copied from the inspected contract"`
}

// ExecuteDataInput supports up to ten calls in a single provider request.
// It validates each call against the free catalogue before using credits.
type ExecuteDataInput struct {
	Calls      []DataCall `json:"calls" jsonschema:"One to ten operation calls; inspect each operation to discover exact input field names"`
	MaxCredits int        `json:"max_credits,omitempty" jsonschema:"Maximum predicted credits for this request; defaults to WEBCTX_ALEXANDRIA_MAX_CREDITS or 100"`
	RequestID  string     `json:"request_id,omitempty" jsonschema:"Optional stable idempotency identifier for retrying a paid request; reuse unchanged if retrying"`
	Raw        bool       `json:"raw,omitempty" jsonschema:"Include the complete original provider response"`
}
