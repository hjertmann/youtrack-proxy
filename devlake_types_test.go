package main

// EndpointStatus classifies a proxy endpoint's implementation state.
type EndpointStatus string

const (
	StatusImplemented EndpointStatus = "implemented"
	StatusStubbed     EndpointStatus = "stubbed"
	StatusMissing     EndpointStatus = "missing"
)

// EndpointReport describes a single endpoint's test result.
type EndpointReport struct {
	Path             string
	Method           string
	Status           EndpointStatus
	ExpectedContract string
	DevLakeSource    string
}
