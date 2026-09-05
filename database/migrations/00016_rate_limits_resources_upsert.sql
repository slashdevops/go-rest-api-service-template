-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- The authz resource rows for the rate-limit endpoints.
--
-- WHY A SEPARATE MIGRATION, when 00008_roles_policies_tables_upsert.sql already carries them.
--
-- That file is what a FRESHLY CREATED database gets. An existing one never re-runs it -- goose
-- tracks versions by number and does not checksum contents -- so without this migration every
-- deployment that already exists would have six endpoints with no resource to authorise against,
-- and every request to them would be refused for a reason nothing explains.
--
-- ON CONFLICT DO NOTHING because a fresh database gets these from 00008 first, and this must then
-- be a no-op rather than a duplicate-key failure at startup.
--
-- No new POLICY rows: the seeded 'Full Access' policy is ('*', '*'), so it already authorises
-- these once the resources exist. Adding narrower policies is an operator's decision, not a
-- migration's.
---------------------------------------------------------------------------------------------------
INSERT INTO resources (id, name, description, action, resource, system) VALUES
('01a03a46-16d4-7831-9c94-a7975a9c4334', 'List rate limits'                                    , 'List the rate-limit rules, with filtering, sorting, partial fields and pagination'                                                                                                                                                                                                                                                      , 'GET'   , '/rate_limits'                                                                                      , TRUE    ),
('01a03a46-16d4-7ad9-b646-0bc67824b38c', 'Create rate limit'                                   , 'Create a rate-limit rule. The target is validated against the endpoint catalogue, so a rule for a route this service does not serve is refused rather than silently protecting nothing'                                                                                                                                                 , 'POST'  , '/rate_limits'                                                                                      , TRUE    ),
('01a03a46-16d4-7b2b-8932-ef9694d8f940', 'Effective rate limits'                               , 'Resolve which rules apply to a method and endpoint, one per scope, most specific first. Resolved with the same function the limiter uses, so it cannot disagree with what is enforced'                                                                                                                                                  , 'GET'   , '/rate_limits/effective'                                                                            , TRUE    ),
('01a03a46-16d4-7b1a-913f-e9e50f9acfa7', 'Delete rate limit'                                   , 'Delete a rate-limit rule. Its windows are removed with it'                                                                                                                                                                                                                                                                              , 'DELETE', '/rate_limits/{rate_limit_id}'                                                                      , TRUE    ),
('01a03a46-16d4-7af9-9f96-d9dc094afd80', 'Get rate limit'                                      , 'Retrieve a rate-limit rule and its windows by unique identifier'                                                                                                                                                                                                                                                                        , 'GET'   , '/rate_limits/{rate_limit_id}'                                                                      , TRUE    ),
('01a03a46-16d4-7b0a-af95-6805d68a37d3', 'Update rate limit'                                   , 'Replace a rate-limit rule. The window set is replaced in full, not merged'                                                                                                                                                                                                                                                              , 'PUT'   , '/rate_limits/{rate_limit_id}'                                                                      , TRUE    )
ON CONFLICT (id) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- These rows are system = TRUE, and tr_restrict_delete_update_on_system_resources rejects both
-- UPDATE and DELETE on them -- so a down migration cannot clear the flag first. It must disable
-- the trigger, delete, and re-enable. Leaving it disabled would silently unprotect every system
-- row in the table.
ALTER TABLE resources DISABLE TRIGGER tr_restrict_delete_update_on_system_resources;

DELETE FROM resources WHERE id IN (
    '01a03a46-16d4-7831-9c94-a7975a9c4334',
    '01a03a46-16d4-7ad9-b646-0bc67824b38c',
    '01a03a46-16d4-7b2b-8932-ef9694d8f940',
    '01a03a46-16d4-7b1a-913f-e9e50f9acfa7',
    '01a03a46-16d4-7af9-9f96-d9dc094afd80',
    '01a03a46-16d4-7b0a-af95-6805d68a37d3'
);

ALTER TABLE resources ENABLE TRIGGER tr_restrict_delete_update_on_system_resources;

-- +goose StatementEnd
