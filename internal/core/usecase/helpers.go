package usecase

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"

	"golang.org/x/crypto/bcrypt"
)

// HashAndSaltPassword hashes and salts the password
// It uses bcrypt to hash the password with a cost of 10.
// The hashed password is returned as a string.
func HashAndSaltPassword(password string, cost ...int) (string, error) {
	var costVal int
	if len(cost) > 0 {
		if cost[0] < bcrypt.MinCost || cost[0] > bcrypt.MaxCost {
			return "", fmt.Errorf("cost value must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}
		costVal = cost[0]
	} else {
		costVal = int(passwordHashCost.Load())
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), costVal)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// ComparePasswords compares the hashed password and the plain password
// It uses bcrypt to compare the hashed password with the plain password.
// It returns true if the passwords match, false otherwise.
func ComparePasswords(hashedPwd string, plainPwd string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(plainPwd))
	return err == nil
}

func CountWords(text string) int {
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Split(bufio.ScanWords)
	count := 0

	for scanner.Scan() {
		count++
	}

	return count
}

var reUUID = regexp.MustCompile(domain.ValidUUIDOrStarRegex)

// convertToSQLRegex replaces UUIDs and * in a resource string with SQL regex patterns.
// It converts UUIDs to a regex pattern that matches any string of characters (.*).
// Example: "projects/123e4567-e89b-12d3-a456-426614174000/details" becomes "projects/.*?/details".
// The function also adds ^ at the beginning and $ at the end of the string to ensure it matches the entire string.
func convertToSQLRegex(resource string) string {
	// https://regex101.com/r/4bn9da/1
	resource = reUUID.ReplaceAllString(resource, "\\{[a-z_]{1,50}\\}")

	resource = `^` + resource + `$`

	return resource
}

// ResourceLimitCheckParams contains parameters for resource limit checking
type ResourceLimitCheckParams struct {
	Ctx             context.Context
	ResourcesLimits ResourcesLimitsServiceConsumer
	Scope           domain.ResourcesLimitsScope
	ResourceType    domain.ResourcesLimitsResourceType
}

// ReserveResourceSlot claims one unit of a resource before it is created.
// It returns [domain.ResourcesLimitsHardLimitReachedError] when the hard limit
// is already reached.
//
// Pair every successful call with [ReleaseResourceSlot] on the failure path of
// the creation that follows:
//
//	if err := ReserveResourceSlot(params); err != nil {
//	    return err
//	}
//
//	if err := ref.repository.Insert(ctx, input); err != nil {
//	    ReleaseResourceSlot(params)
//	    return err
//	}
//
// Reserving first is deliberate. It means an interrupted create leaves the
// counter one too high — which refuses a later request — rather than one too
// low, which would hand out capacity that was never licensed. Over-counting is
// recoverable by reconciliation; under-counting is a silent loss.
//
// This replaces the older CheckResourceLimitsBeforeCreation /
// IncrementResourceUsageAfterCreation pair, which performed the check and the
// increment as two unlocked statements around the insert. Concurrent callers
// all read the same pre-insert count and all passed, so N simultaneous requests
// could exceed a hard limit by up to N-1.
func ReserveResourceSlot(params ResourceLimitCheckParams) error {
	return params.ResourcesLimits.ReserveUsage(params.Ctx, params.Scope, params.ResourceType)
}

// ReleaseResourceSlot returns a reservation that was claimed but not used,
// because the creation it was claimed for failed.
//
// It deliberately does not surface its error to the caller: the request has
// already failed for another reason, and replacing that reason with a
// bookkeeping error would hide what actually went wrong. A failed release
// leaves the counter high, which is the safe direction and is what
// reconciliation exists to repair.
func ReleaseResourceSlot(params ResourceLimitCheckParams) {
	if err := params.ResourcesLimits.DecrementUsage(params.Ctx, params.Scope, params.ResourceType); err != nil {
		scopeID := params.Scope.ID
		if scopeID == nil {
			scopeID = new(uuid.Nil())
		}

		slog.Error(
			"failed to release a resource reservation; the counter is now one too high until reconciliation runs",
			"scope_type", params.Scope.Type,
			"scope_id", scopeID,
			"resource_type", params.ResourceType,
			"error", err,
		)
	}
}

// WarnOnSoftLimit logs when a scope is at or past its soft limit. It is a
// reporting aid only and never blocks; the hard limit is enforced inside
// [ReserveResourceSlot].
func WarnOnSoftLimit(params ResourceLimitCheckParams) {
	status, err := params.ResourcesLimits.CheckUsage(params.Ctx, params.Scope, params.ResourceType)
	if err != nil || !status.SoftLimitReached {
		return
	}

	scopeID := params.Scope.ID
	if scopeID == nil {
		scopeID = new(uuid.Nil())
	}

	slog.Warn(
		fmt.Sprintf("soft limit reached: scope type %s, scope ID %v, resource type %s",
			params.Scope.Type, scopeID, params.ResourceType),
		"soft_limit", status.SoftLimit,
		"hard_limit", status.HardLimit,
		"current_usage", status.CurrentUsage,
	)
}

// passwordHashCost is the bcrypt cost for new hashes when a caller does not
// name one. Set once at startup from authn.password.bcrypt.cost; the library
// default (10) is the fallback if nothing ever sets it.
var passwordHashCost = func() *atomic.Int64 {
	var v atomic.Int64
	v.Store(int64(bcrypt.DefaultCost))
	return &v
}()

// SetPasswordHashCost sets the bcrypt cost new password hashes use.
func SetPasswordHashCost(cost int) error {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return fmt.Errorf("bcrypt cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}

	passwordHashCost.Store(int64(cost))

	return nil
}
