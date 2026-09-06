// Package safedial is the outbound address guard.
//
// Operators supply URLs the service then dials: an LLM engine's api_endpoint
// on every embed and generate, an identity provider's issuer on discovery.
// Nothing checked the host. A URL naming 169.254.169.254, localhost or an
// address on the service's own network turned a grant on "create engine"
// into a request from inside the perimeter, with the answer relayed back.
//
// The check runs in the dialer's Control hook, after DNS, on the address
// actually being connected to. Checking the URL's host at write time would
// answer for the name, not the address, and a name is free to resolve
// somewhere else later (DNS rebinding); a redirect is dialled through the
// same hook. Link-local addresses -- the cloud metadata range -- are refused
// always. Loopback and private ranges are refused unless the operator says
// the deployment talks to engines there (a local Ollama, an on-premises
// provider), which the dev stack does.
package safedial
