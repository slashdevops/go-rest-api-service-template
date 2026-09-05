package domain

import "github.com/slashdevops/qfv"

// qfv parsers are immutable and safe for concurrent reuse, so each entity's
// sort/filter/fields parser is built once at package init and shared across
// requests rather than reconstructed on every Validate call. See the qfv v1
// docs: "Parsers are safe to build once and reuse across goroutines."
//
//nolint:gochecknoglobals // Immutable, concurrency-safe parser singletons.
var (
	IDPsSortParser   = qfv.NewSortParser(IDPsSortFields)
	IDPsFilterParser = qfv.NewFilterParser(IDPsFilterFields)
	IDPsFieldsParser = qfv.NewFieldsParser(IDPsPartialFields)

	IDPTypesSortParser   = qfv.NewSortParser(IDPTypesSortFields)
	IDPTypesFilterParser = qfv.NewFilterParser(IDPTypesFilterFields)
	IDPTypesFieldsParser = qfv.NewFieldsParser(IDPTypesPartialFields)

	PoliciesSortParser   = qfv.NewSortParser(PoliciesSortFields)
	PoliciesFilterParser = qfv.NewFilterParser(PoliciesFilterFields)
	PoliciesFieldsParser = qfv.NewFieldsParser(PoliciesPartialFields)

	ProductsSortParser   = qfv.NewSortParser(ProductsSortFields)
	ProductsFilterParser = qfv.NewFilterParser(ProductsFilterFields)
	ProductsFieldsParser = qfv.NewFieldsParser(ProductsPartialFields)

	ProjectSortParser   = qfv.NewSortParser(ProjectSortFields)
	ProjectFilterParser = qfv.NewFilterParser(ProjectFilterFields)
	ProjectFieldsParser = qfv.NewFieldsParser(ProjectPartialFields)

	ResourcesSortParser   = qfv.NewSortParser(ResourcesSortFields)
	ResourcesFilterParser = qfv.NewFilterParser(ResourcesFilterFields)
	ResourcesFieldsParser = qfv.NewFieldsParser(ResourcesPartialFields)

	ResourcesLimitsSortParser   = qfv.NewSortParser(ResourcesLimitsSortFields)
	ResourcesLimitsFilterParser = qfv.NewFilterParser(ResourcesLimitsFilterFields)
	ResourcesLimitsFieldsParser = qfv.NewFieldsParser(ResourcesLimitsPartialFields)

	RateLimitsSortParser   = qfv.NewSortParser(RateLimitsSortFields)
	RateLimitsFilterParser = qfv.NewFilterParser(RateLimitsFilterFields)
	RateLimitsFieldsParser = qfv.NewFieldsParser(RateLimitsPartialFields)

	RolesSortParser   = qfv.NewSortParser(RolesSortFields)
	RolesFilterParser = qfv.NewFilterParser(RolesFilterFields)
	RolesFieldsParser = qfv.NewFieldsParser(RolesPartialFields)

	UsersSortParser   = qfv.NewSortParser(UsersSortFields)
	UsersFilterParser = qfv.NewFilterParser(UsersFilterFields)
	UsersFieldsParser = qfv.NewFieldsParser(UsersPartialFields)
)
