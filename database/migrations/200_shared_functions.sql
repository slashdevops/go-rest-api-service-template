-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- The system-row guard, once.
--
-- Several tables carry a `system BOOLEAN` column marking rows that ship with the product: roles,
-- resources, policies, resource limits, rate limits, and so on. None of them
-- may be modified or removed through the API, and the guard is a trigger rather than an
-- application-level check on purpose — rows drift precisely when something bypasses the service
-- (direct SQL, an operator cleanup, a CASCADE from another table), and a trigger holds for all of
-- those.
--
-- This used to be twenty copies of the same seventeen lines, one function per table, differing only
-- in the table name inside the message. TG_TABLE_NAME supplies that, so one function serves them
-- all. The per-table TRIGGERS keep their own names, which is what matters: the seed migrations rely
-- on `ALTER TABLE x DISABLE TRIGGER tr_restrict_delete_update_on_system_x` to delete their own rows
-- on the way down, and that idiom is untouched.
--
-- Three of the copies had drifted from the table they guarded and reported a name that did not
-- exist — `permissions`
-- for resources. TG_TABLE_NAME cannot drift.
--
-- This lives at 200 so that it is created before the first table that needs it and dropped after
-- the last one is gone: goose applies Up in ascending order and Down in descending order, so a
-- shared object has to sort below everything that depends on it.
---------------------------------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION fn_restrict_delete_update_on_system()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' AND OLD.system THEN
        RAISE EXCEPTION 'System % cannot be deleted.', TG_TABLE_NAME;
    ELSIF TG_OP = 'UPDATE' AND OLD.system THEN
        RAISE EXCEPTION 'System % cannot be updated.', TG_TABLE_NAME;
    ELSIF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSIF TG_OP = 'UPDATE' THEN
        RETURN NEW;
    END IF;

    -- Every trigger using this function is BEFORE DELETE OR UPDATE. Returning NULL from a BEFORE
    -- row trigger CANCELS the operation, so an unexpected event must not fall off the end.
    RAISE EXCEPTION 'fn_restrict_delete_update_on_system attached to an unsupported event: % on %',
        TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS fn_restrict_delete_update_on_system();

-- +goose StatementEnd
