// Package driving defines the driving (primary) ports that adapters
// such as the HTTP handlers consume. Each entity has its own
// interface (Authn, Users, Projects, …) capturing only the use-case
// operations the adapter needs (Interface Segregation).
//
// The use-case implementations live in internal/service today; the
// composition root in internal/app wires them in. Adapters never
// import internal/service directly — they depend only on the ports
// in this package.
package driving
