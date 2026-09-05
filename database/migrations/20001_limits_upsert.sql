-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- Default resource limits.
--
-- Without rows in resources_limits every lookup falls through to the -1/-1 fallback in
-- repositorypg.ResourcesLimitsRepository.CheckUsage, which the service reads as "unlimited" — so
-- nothing is limited at all. These rows are what turns the mechanism on.
--
-- scope_id = the zero UUID marks the *default* row for a scope type. CheckUsage resolves in three
-- steps: a limit for the exact scope_id, then this default row, then the unlimited fallback.
--
-- system = FALSE on purpose. fn_restrict_delete_update_on_system rejects both
-- UPDATE and DELETE on system rows, so marking these TRUE would freeze the defaults permanently —
-- an operator could never raise a customer's ceiling without disabling the trigger. These are
-- defaults, not immutable facts. They become the free tier once limits are sourced from the
-- signed license file at startup.
--
-- On the values: an earlier draft proposed users 2/5, idps 3/6, projects 10/12 and products
-- 20/25. Those are demo numbers and cannot go live as-is — they would cap a whole deployment at
-- five users, and a pagination test that creates a dozen rows for one owner would fail against
-- them. The numbers below are deliberately generous: they turn the *mechanism* on without capping
-- anything real. The actual ceilings arrive with the signed license file, which is free to lower
-- them.
---------------------------------------------------------------------------------------------------

-- System-wide limits: how much of this deployment can exist in total.
INSERT INTO resources_limits (id, scope_type, scope_id, resource_type, soft_limit, hard_limit, system) VALUES
  ('01a00bb3-bd46-7e67-923d-60b9503781d6', 'system', '00000000-0000-0000-0000-000000000000', 'users', 500, 1000, FALSE),
  -- idps is generous for the same reason as users: it is a system-wide counter that only ever
  -- climbs, because anything that removes an IdP outside the service's delete path (direct SQL,
  -- an admin cleanup, a test helper) never decrements it. Until reconciliation exists, a tight
  -- ceiling here becomes a permanent lockout rather than a limit.
  ('01a00bb3-bd47-705d-9f5a-5f45dc599aed', 'system', '00000000-0000-0000-0000-000000000000', 'idps',  500, 1000, FALSE)
ON CONFLICT (scope_type, scope_id, resource_type) DO NOTHING;

-- Per-user defaults: applied to every user that has no limit row of its own.
INSERT INTO resources_limits (id, scope_type, scope_id, resource_type, soft_limit, hard_limit, system) VALUES
  ('01a00bb3-bd47-7075-a5d2-23ada1f3f876', 'user', '00000000-0000-0000-0000-000000000000', 'projects',  100, 120, FALSE)
ON CONFLICT (scope_type, scope_id, resource_type) DO NOTHING;

-- Per-project defaults: applied to every project that has no limit row of its own.
INSERT INTO resources_limits (id, scope_type, scope_id, resource_type, soft_limit, hard_limit, system) VALUES
  ('01a00bb3-bd47-7094-b303-7c656d40e764', 'project', '00000000-0000-0000-0000-000000000000', 'products',         200, 250, FALSE)
ON CONFLICT (scope_type, scope_id, resource_type) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Deleted by id rather than by scope, so an operator-created limit that happens to share a
-- (scope_type, scope_id, resource_type) key is never removed by rolling this migration back.
-- No trigger juggling is needed here because these rows are not system rows.
DELETE FROM resources_limits WHERE id IN (
  '01a00bb3-bd46-7e67-923d-60b9503781d6',
  '01a00bb3-bd47-705d-9f5a-5f45dc599aed',
  '01a00bb3-bd47-7075-a5d2-23ada1f3f876',
  '01a00bb3-bd47-7084-89a7-94abe8cdadbd',
  '01a00bb3-bd47-7094-b303-7c656d40e764',
  '01a00bb3-bd47-70a0-a4e0-a8c31c4e2322',
  '01a00bb3-bd47-70af-918f-8cbf306215a0'
);

-- +goose StatementEnd
