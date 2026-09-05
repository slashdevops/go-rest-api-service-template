package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// loadResourceLimitsSigningKeys returns the key pair used to sign and verify the
// usage counters.
//
// It defaults to the JWT pair, so an existing deployment upgrades with no change
// in behaviour. Pointing it at its own pair decouples the two, which matters
// because they authenticate different things and have different rotation
// pressures: a JWT key gets rotated after a token leak, and doing that today
// invalidates every stored usage signature as a side effect.
//
// "Defaults to the JWT pair" means the *configured* JWT pair, not the default
// JWT filename. Those are different things whenever an operator points
// --authn.private.key.file somewhere else, which every deployment here does —
// the development environment passes ./certs/jwt.key. Reading the literal
// default filename instead made the service fail to start with
// "open jwt.key: no such file or directory", so the settings being left alone
// has to mean "reuse what authn already loaded".
func (a *App) loadResourceLimitsSigningKeys(jwt *authKeys) (privateKey, publicKey []byte, err error) {
	privateKeyFile := a.configs.ResourcesLimits.SigningPrivateKeyFile.Value.Name()
	publicKeyFile := a.configs.ResourcesLimits.SigningPublicKeyFile.Value.Name()

	usingDefaults := privateKeyFile == config.DefaultResourcesLimitsSigningPrivateKeyFile.Name() &&
		publicKeyFile == config.DefaultResourcesLimitsSigningPublicKeyFile.Name()

	if usingDefaults {
		slog.Debug("resource limits signing keys follow the JWT keys",
			"private", a.configs.Authn.PrivateKeyFile.Value.Name())

		return jwt.jwtPrivateKey, jwt.jwtPublicKey, nil
	}

	slog.Debug("loading resource limits signing keys", "private", privateKeyFile, "public", publicKeyFile)

	privateKey, err = os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading resource limits signing private key file: %w", err)
	}

	publicKey, err = os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading resource limits signing public key file: %w", err)
	}

	// A deployment that has just split the keys has counters signed by the old
	// one. They will all fail verification, which refuses writes for every scope
	// until a reconciliation pass re-signs them — so say so at the one moment
	// somebody is watching, rather than letting it surface as "nothing works".
	if !bytes.Equal(privateKey, jwt.jwtPrivateKey) && !a.configs.ResourcesLimits.ReconcileOnStart.Value {
		slog.Warn(
			"resource usage counters are signed with a key of their own, and reconciliation is disabled",
			"what", "counters signed by a previous key will fail verification and refuse writes for their scope",
			"fix", "start once with --resources.limits.reconcile.on.start=true to recount and re-sign every counter",
			"private_key_file", privateKeyFile,
		)
	}

	return privateKey, publicKey, nil
}

// reconcileResourceUsage recomputes every resource-usage counter from the tables
// it tracks, correcting drift left behind by anything that removed a resource
// without going through the service's delete path.
//
// It runs before the HTTP server accepts traffic, so requests are enforced
// against repaired counters rather than racing the repair.
//
// Off by default — see [config.DefaultResourcesLimitsReconcileOnStart] for why.
// A failure here is logged and startup continues: the counters are no worse than
// they were, and refusing to boot over a bookkeeping pass would turn a
// recoverable inconsistency into an outage.
func (a *App) reconcileResourceUsage(ctx context.Context) {
	if !a.configs.ResourcesLimits.ReconcileOnStart.Value {
		return
	}

	if a.services.ResourcesLimits == nil {
		slog.Error("resource usage reconciliation skipped", "reason", "resources limits service is not initialised")
		return
	}

	start := time.Now()
	slog.Info("reconciling resource usage counters")

	corrected, err := a.services.ResourcesLimits.ReconcileAll(ctx)
	if err != nil {
		slog.Error("resource usage reconciliation failed; continuing with the counters as they are", "error", err)
		return
	}

	if corrected == 0 {
		slog.Info("resource usage reconciliation finished, every counter already matched",
			"duration", time.Since(start))

		return
	}

	// Worth a warning rather than an info: a corrected counter means something
	// changed resources without telling the limits subsystem.
	slog.Warn(
		"resource usage reconciliation corrected counters",
		"corrected", corrected,
		"duration", time.Since(start),
	)
}
