package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ProjectMembershipChecker answers whether a user may act on a project.
type ProjectMembershipChecker interface {
	Membership(ctx context.Context, projectID, userID uuid.UUID) (domain.ProjectMembership, error)
}

// ProjectIDPathValue is the path wildcard every project-scoped route uses.
const ProjectIDPathValue = "project_id"

// RequireProjectMembership refuses a project-scoped request from a caller who
// is neither a member of that project nor an administrator.
//
// The authorization check before this one answers "may this user call GET
// /projects/{any}/embedding_configs", and that is all it can answer: OPA sees
// (user, method, path) and expands the "*" in a grant to any uuid. So one
// project-scoped grant used to open every project -- measured 2026-09-06,
// user A with a grant on /projects/*/embedding_configs listed user B's
// configs by id. Membership is data the database owns; it is asked here,
// once per request, and the handler never has to know.
//
// It keys on the route's project_id path value and does nothing on a route
// that has none, so it sits in the shared authenticated chain. It runs AFTER
// CheckAuthz on purpose: a caller with no grant at all still gets 403; only a
// caller who is authorised for the route but not for this project gets 404 --
// the same answer a missing project gets, so the refusal does not confirm the
// project exists, which is what the projects handler already does.
//
// An administrator is admitted to every project. That is a bypass of
// membership, and it is logged as one, with the project and the user.
func RequireProjectMembership(checker ProjectMembershipChecker) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.PathValue(ProjectIDPathValue)
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}

			projectID, err := uuid.Parse(raw)
			if err != nil {
				// The handler validates the id and answers 400 with its own
				// wording; an unparsable id cannot name a project to refuse.
				next.ServeHTTP(w, r)
				return
			}

			claims, _ := r.Context().Value(JwtClaims).(map[string]any)
			subStr, _ := claims["sub"].(string)

			userID, err := uuid.Parse(subStr)
			if err != nil {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "invalid sub claim")
				return
			}

			membership, err := checker.Membership(r.Context(), projectID, userID)
			if err != nil {
				// Fail closed. An unreachable membership store must not admit
				// anyone; the same rule as the token denylist and CheckAuthz.
				respond.WriteInternalError(w, r, err)
				return
			}

			switch membership {
			case domain.ProjectMembershipAdmin:
				slog.Info("project membership bypassed by an administrator",
					"request_id", respond.RequestIDFrom(r.Context()),
					"project.id", projectID.String(),
					"user.id", userID.String(),
					"method", r.Method,
					"path", r.URL.Path,
				)
			case domain.ProjectMembershipMember:
			default:
				slog.Warn("project-scoped request refused: caller is not a member",
					"request_id", respond.RequestIDFrom(r.Context()),
					"project.id", projectID.String(),
					"user.id", userID.String(),
					"method", r.Method,
					"path", r.URL.Path,
				)
				respond.WriteJSONMessage(w, r, http.StatusNotFound, (&domain.ProjectNotFoundError{ID: projectID}).Error())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
