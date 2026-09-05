// Package repository defines the driven persistence ports that
// use-cases consume. Each entity has its own interface (Users,
// Languages, …) that captures only the storage operations the
// corresponding use-case needs (Interface Segregation).
//
// Concrete implementations live in internal/repository (today; will
// move to internal/adapter/driven/repositorypg in Phase 4).
// The composition root in internal/app wires them in.
package repository
