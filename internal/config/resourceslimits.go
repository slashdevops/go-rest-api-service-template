package config

import "os"

// DefaultResourcesLimitsSigning{Private,Public}KeyFile point at the JWT key pair
// on purpose.
//
// The usage signature and the JWT signature authenticate completely different
// things and should not share a key — that is what these settings exist to fix.
// But defaulting them to a *new* file would rotate the key for every existing
// deployment on upgrade, and rotating this key invalidates every stored
// signature at once: every counter would fail verification and writes would be
// refused for every scope until a reconciliation pass re-signed them.
//
// So the split is opt-in. Point these at their own key pair when you are ready
// to run the rotation, which is:
//
//  1. generate the pair (see docs/certificates/certificates.md)
//  2. set these two settings
//  3. start once with --resources.limits.reconcile.on.start=true, which
//     recounts and re-signs every counter with the new key
//  4. turn reconciliation back off if you want it off
//
// Skipping step 3 leaves every counter signed by a key the service no longer
// holds, which reads as tampering.
var (
	DefaultResourcesLimitsSigningPrivateKeyFile = FileVar{os.NewFile(0, "jwt.key"), os.O_RDONLY}
	DefaultResourcesLimitsSigningPublicKeyFile  = FileVar{os.NewFile(0, "jwt.pub"), os.O_RDONLY}
)

const (
	// DefaultResourcesLimitsReconcileOnStart keeps reconciliation opt-in.
	//
	// Reconciliation rewrites usage counters from the resource tables, so
	// turning it on changes numbers that enforcement then acts on. Two reasons
	// it does not default to true yet:
	//
	//   - Projects are counted by membership, because the schema records no
	//     owner. A user linked to a project they did not create would start
	//     counting it against their own limit the first time a reconciliation
	//     runs. That is a behaviour change, not a repair, and it should be a
	//     deliberate choice until projects carry a created_by column.
	//   - It walks every counter in the database. That is cheap for a handful
	//     of tenants and not obviously cheap for thousands, and startup is a
	//     bad place to discover which one you have.
	//
	// An operator who knows their deployment does not share projects can switch
	// it on and have drift repaired on every boot.
	DefaultResourcesLimitsReconcileOnStart = false
)

// ResourcesLimitsConfig configures the resource-limits subsystem.
type ResourcesLimitsConfig struct {
	// ReconcileOnStart recomputes every usage counter from its resource table
	// during startup, correcting drift and re-signing the result.
	//
	// Drift only ever moves one way — upward, toward refusing creations a tenant
	// is entitled to — because anything that removes a resource outside the
	// service's delete path leaves the counter high. Because counters are
	// signed, this is the only way to repair them: a hand-written value fails
	// verification.
	ReconcileOnStart Field[bool]

	// SigningPrivateKeyFile and SigningPublicKeyFile are the EC key pair used to
	// sign and verify the usage counters, keeping them tamper-evident.
	//
	// These default to the JWT key pair for backwards compatibility. Giving them
	// their own pair means a JWT-key rotation — after a token leak, say — no
	// longer invalidates every usage signature in the database as a side effect.
	SigningPrivateKeyFile Field[FileVar]
	SigningPublicKeyFile  Field[FileVar]
}

// NewResourcesLimitsConfig returns the resource-limits configuration defaults.
func NewResourcesLimitsConfig() *ResourcesLimitsConfig {
	return &ResourcesLimitsConfig{
		ReconcileOnStart: NewField(
			"resources.limits.reconcile.on.start",
			"RESOURCES_LIMITS_RECONCILE_ON_START",
			"Recompute every resource usage counter from its source table during startup, repairing drift",
			DefaultResourcesLimitsReconcileOnStart,
		),
		SigningPrivateKeyFile: NewField(
			"resources.limits.signing.private.key.file",
			"RESOURCES_LIMITS_SIGNING_PRIVATE_KEY_FILE",
			"EC private key used to sign resource usage counters. Defaults to the JWT key; give it its own pair to decouple the two",
			DefaultResourcesLimitsSigningPrivateKeyFile,
		),
		SigningPublicKeyFile: NewField(
			"resources.limits.signing.public.key.file",
			"RESOURCES_LIMITS_SIGNING_PUBLIC_KEY_FILE",
			"EC public key used to verify resource usage counters. Defaults to the JWT key; give it its own pair to decouple the two",
			DefaultResourcesLimitsSigningPublicKeyFile,
		),
	}
}

// ParseEnvVars overrides the configured values with environment variables.
func (ref *ResourcesLimitsConfig) ParseEnvVars() {
	ref.ReconcileOnStart.Value = GetEnv(ref.ReconcileOnStart.EnVarName, ref.ReconcileOnStart.Value)
	ref.SigningPrivateKeyFile.Value = GetEnv(ref.SigningPrivateKeyFile.EnVarName, ref.SigningPrivateKeyFile.Value)
	ref.SigningPublicKeyFile.Value = GetEnv(ref.SigningPublicKeyFile.EnVarName, ref.SigningPublicKeyFile.Value)
}

// Validate reports configuration errors. A boolean switch has none, but the
// method keeps this config usable with [Validate] alongside the others.
func (ref *ResourcesLimitsConfig) Validate() error {
	return nil
}
